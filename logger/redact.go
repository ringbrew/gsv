package logger

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

const (
	DefaultMaxStringLen = 4096
	DefaultMaxDepth     = 8

	maskText        = "***"
	truncatedSuffix = "...(truncated)"
	unparsableText  = "***(unparsable, redacted)"

	// sensitive keys at least this long also match as a suffix, e.g. "password" hits "user_password"
	minSuffixKeyLen = 5
)

var defaultSensitiveKeys = []string{
	"password", "passwd", "pwd", "oldpassword", "newpassword",
	"token", "accesstoken", "refreshtoken", "idtoken",
	"authorization", "cookie", "setcookie",
	"secret", "appsecret", "clientsecret", "apikey", "accesskey", "secretkey", "privatekey",
	"signature", "sign",
	"idcard", "idcardno", "idno",
	"phone", "mobile", "telephone", "email",
	"bankcard", "cardno",
	"credential", "session", "sessionid",
	"otp", "smscode", "verifycode",
}

var defaultJSONKeys = []string{"body", "req", "resp", "response", "payload"}

// Redactor masks sensitive values in log extras by key name.
//
// Redaction is idempotent: running a Redactor over its own output changes
// nothing, so an entry passing through both the package level functions and
// a Logger implementation is safe. A custom mask set by WithMask must keep
// this property.
type Redactor struct {
	exact         map[string]struct{}
	jsonKeys      map[string]struct{}
	trie          *keyTrie
	maxStringLen  int
	maxDepth      int
	mask          func(string) string
	deepMarshaler bool

	// reflect.Type -> bool, whether values of the type may hold sensitive keys
	types sync.Map
}

type RedactorOption func(*Redactor)

// WithSensitiveKeys adds keys to the sensitive key set.
func WithSensitiveKeys(keys ...string) RedactorOption {
	return func(r *Redactor) {
		for _, k := range keys {
			r.exact[normalizeKey(k)] = struct{}{}
		}
	}
}

// WithoutSensitiveKeys removes keys from the sensitive key set, including
// built-in ones.
func WithoutSensitiveKeys(keys ...string) RedactorOption {
	return func(r *Redactor) {
		for _, k := range keys {
			delete(r.exact, normalizeKey(k))
		}
	}
}

// WithJSONKeys adds extra keys whose string values are JSON text to be parsed
// and redacted.
func WithJSONKeys(keys ...string) RedactorOption {
	return func(r *Redactor) {
		for _, k := range keys {
			r.jsonKeys[normalizeKey(k)] = struct{}{}
		}
	}
}

// WithMaxStringLen truncates top level string values of extra longer than n
// bytes. n <= 0 disables truncation.
func WithMaxStringLen(n int) RedactorOption {
	return func(r *Redactor) {
		r.maxStringLen = n
	}
}

// WithMaxDepth limits how deep nested values are scanned. Values below the
// limit are left untouched.
func WithMaxDepth(n int) RedactorOption {
	return func(r *Redactor) {
		r.maxDepth = n
	}
}

// WithMask replaces the default mask. fn must satisfy fn(fn(s)) == fn(s).
func WithMask(fn func(string) string) RedactorOption {
	return func(r *Redactor) {
		if fn != nil {
			r.mask = fn
		}
	}
}

// WithDeepMarshalerScan makes the Redactor walk into types implementing
// json.Marshaler or encoding.TextMarshaler. By default they are treated as
// opaque values, because rewriting them would change their encoded form.
func WithDeepMarshalerScan() RedactorOption {
	return func(r *Redactor) {
		r.deepMarshaler = true
	}
}

func NewRedactor(opts ...RedactorOption) *Redactor {
	r := &Redactor{
		exact:        make(map[string]struct{}),
		jsonKeys:     make(map[string]struct{}),
		maxStringLen: DefaultMaxStringLen,
		maxDepth:     DefaultMaxDepth,
		mask:         DefaultMask,
	}
	WithSensitiveKeys(defaultSensitiveKeys...)(r)
	WithJSONKeys(defaultJSONKeys...)(r)
	for _, o := range opts {
		o(r)
	}
	r.trie = &keyTrie{}
	for k := range r.exact {
		node := r.trie.insert(k)
		node.sensitive = true
		node.suffix = len(k) >= minSuffixKeyLen
	}
	for k := range r.jsonKeys {
		r.trie.insert(k).json = true
	}
	return r
}

// keyTrie holds normalized keys reversed, so one backward pass over a key
// answers both exact and suffix matches.
type keyTrie struct {
	next      [36]*keyTrie
	other     map[byte]*keyTrie
	sensitive bool
	suffix    bool
	json      bool
}

func trieIndex(c byte) int {
	switch {
	case 'a' <= c && c <= 'z':
		return int(c - 'a')
	case '0' <= c && c <= '9':
		return int(c-'0') + 26
	}
	return -1
}

func (t *keyTrie) child(c byte) *keyTrie {
	if i := trieIndex(c); i >= 0 {
		return t.next[i]
	}
	return t.other[c]
}

func (t *keyTrie) insert(key string) *keyTrie {
	n := t
	for i := len(key) - 1; i >= 0; i-- {
		c := key[i]
		next := n.child(c)
		if next == nil {
			next = &keyTrie{}
			if idx := trieIndex(c); idx >= 0 {
				n.next[idx] = next
			} else {
				if n.other == nil {
					n.other = make(map[byte]*keyTrie)
				}
				n.other[c] = next
			}
		}
		n = next
	}
	return n
}

var redactor atomic.Pointer[Redactor]

func init() {
	redactor.Store(NewRedactor())
}

// SetRedactor replaces the global Redactor. nil disables redaction.
func SetRedactor(r *Redactor) {
	redactor.Store(r)
}

func GetRedactor() *Redactor {
	return redactor.Load()
}

// RedactedExtra returns entry.Extra with sensitive values masked by the global
// Redactor. The original map is never modified; it is returned as is when
// nothing needs to change.
func (entry *LogEntry) RedactedExtra() map[string]interface{} {
	return redactor.Load().RedactExtra(entry.Extra)
}

// RedactValue masks sensitive keys inside v with the global Redactor.
func RedactValue(v interface{}) interface{} {
	return redactor.Load().RedactValue(v)
}

// RedactQuery masks sensitive parameters of a URL query or form body with
// the global Redactor.
func RedactQuery(raw string) string {
	return redactor.Load().RedactQuery(raw)
}

// DefaultMask keeps a few leading and trailing characters of long values and
// hides short ones entirely. Its output is a fixed point of itself.
func DefaultMask(s string) string {
	n := utf8.RuneCountInString(s)
	switch {
	case n == 0:
		return s
	case n <= 6:
		return maskText
	case n <= 10:
		return keepEnds(s, 1, 1, "*****")
	default:
		return keepEnds(s, 3, 2, "******")
	}
}

func keepEnds(s string, head, tail int, stars string) string {
	i := 0
	for k := 0; k < head; k++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	j := len(s)
	for k := 0; k < tail; k++ {
		_, size := utf8.DecodeLastRuneInString(s[:j])
		j -= size
	}
	if s[i:j] == stars {
		return s
	}
	return s[:i] + stars + s[j:]
}

// RedactExtra returns extra with sensitive values masked, copying the map
// only when something changes.
func (r *Redactor) RedactExtra(extra map[string]interface{}) map[string]interface{} {
	if r == nil || len(extra) == 0 {
		return extra
	}
	var out map[string]interface{}
	for k, v := range extra {
		nv, changed := r.redactTop(k, v)
		if !changed {
			continue
		}
		if out == nil {
			out = make(map[string]interface{}, len(extra))
			for k2, v2 := range extra {
				out[k2] = v2
			}
		}
		out[k] = nv
	}
	if out == nil {
		return extra
	}
	return out
}

// RedactValue masks sensitive keys inside v. v itself is never modified.
func (r *Redactor) RedactValue(v interface{}) interface{} {
	if r == nil {
		return v
	}
	if nv, changed := r.redactValue(v, 1); changed {
		return nv
	}
	return v
}

// RedactQuery masks values of sensitive parameters in "a=1&b=2" text.
func (r *Redactor) RedactQuery(raw string) string {
	if r == nil || raw == "" || !r.queryHasSensitive(raw) {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw))
	rest := raw
	for {
		part, sep, more := cutQueryPart(rest)
		key, value, hasValue := strings.Cut(part, "=")
		if hasValue && r.isSensitive(unescapeKey(key)) {
			b.WriteString(key)
			b.WriteByte('=')
			b.WriteString(r.mask(value))
		} else {
			b.WriteString(part)
		}
		if !more {
			break
		}
		b.WriteByte(sep)
		rest = rest[len(part)+1:]
	}
	return b.String()
}

func (r *Redactor) queryHasSensitive(raw string) bool {
	rest := raw
	for {
		part, _, more := cutQueryPart(rest)
		key, _, _ := strings.Cut(part, "=")
		if r.isSensitive(unescapeKey(key)) {
			return true
		}
		if !more {
			return false
		}
		rest = rest[len(part)+1:]
	}
}

func cutQueryPart(s string) (part string, sep byte, more bool) {
	if i := strings.IndexAny(s, "&;"); i >= 0 {
		return s[:i], s[i], true
	}
	return s, 0, false
}

func unescapeKey(key string) string {
	if strings.ContainsAny(key, "%+") {
		if k, err := url.QueryUnescape(key); err == nil {
			return k
		}
	}
	return key
}

func (r *Redactor) redactTop(key string, v interface{}) (interface{}, bool) {
	sensitive, jsonKey := r.classifyKey(key)
	if sensitive {
		return r.maskValue(v)
	}
	s, ok := v.(string)
	if !ok {
		return r.redactValue(v, 1)
	}
	ns := s
	changed := false
	if jsonKey {
		ns, changed = r.redactJSONText(ns)
	}
	if t, truncated := r.truncate(ns); truncated {
		ns, changed = t, true
	}
	if !changed {
		// returning v avoids boxing s into a new interface
		return v, false
	}
	return ns, true
}

func (r *Redactor) truncate(s string) (string, bool) {
	limit := r.maxStringLen
	if limit <= 0 || len(s) <= limit {
		return s, false
	}
	cut := limit - len(truncatedSuffix)
	if cut < 0 {
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncatedSuffix, true
}

func (r *Redactor) redactJSONText(s string) (string, bool) {
	t := strings.TrimLeft(s, " \t\r\n")
	if t == "" || (t[0] != '{' && t[0] != '[') || !r.jsonHasSensitiveKey(t) {
		return s, false
	}
	dec := json.NewDecoder(strings.NewReader(t))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return unparsableText, true
	}
	nv, changed := r.redactValue(v, 1)
	if !changed {
		return s, false
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(nv); err != nil {
		return unparsableText, true
	}
	return strings.TrimSuffix(buf.String(), "\n"), true
}

// jsonHasSensitiveKey scans JSON text once without allocating and reports
// whether any object key is sensitive. Keys with escape sequences are
// reported conservatively.
func (r *Redactor) jsonHasSensitiveKey(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '"' {
			continue
		}
		j := i + 1
		escaped := false
		for j < len(s) && s[j] != '"' {
			if s[j] == '\\' {
				escaped = true
				j++
			}
			j++
		}
		if j >= len(s) {
			return false
		}
		k := j + 1
		for k < len(s) && (s[k] == ' ' || s[k] == '\t' || s[k] == '\r' || s[k] == '\n') {
			k++
		}
		if k < len(s) && s[k] == ':' {
			if escaped || r.isSensitive(s[i+1:j]) {
				return true
			}
		}
		i = j
	}
	return false
}

// maskValue hides v but keeps its JSON type: a field already mapped as a
// number or object in Elasticsearch would reject the whole document if it
// suddenly received a string.
func (r *Redactor) maskValue(v interface{}) (interface{}, bool) {
	switch x := v.(type) {
	case nil:
		return nil, false
	case string:
		m := r.mask(x)
		return m, m != x
	case []string:
		return r.maskStrings(x)
	case json.Number:
		return json.Number("0"), x != "0"
	}
	return r.maskReflect(reflect.ValueOf(v))
}

func (r *Redactor) maskStrings(ss []string) (interface{}, bool) {
	var out []string
	for i, s := range ss {
		m := r.mask(s)
		if m == s {
			continue
		}
		if out == nil {
			out = make([]string, len(ss))
			copy(out, ss)
		}
		out[i] = m
	}
	if out == nil {
		return ss, false
	}
	return out, true
}

func (r *Redactor) maskReflect(fv reflect.Value) (interface{}, bool) {
	for fv.Kind() == reflect.Interface || fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return nil, false
		}
		if fv.Kind() == reflect.Ptr && hasMarshaler(fv.Type()) {
			break
		}
		fv = fv.Elem()
	}
	if hasMarshaler(fv.Type()) && fv.CanInterface() {
		return r.maskEncoded(fv.Interface())
	}
	switch fv.Kind() {
	case reflect.String:
		s := fv.String()
		m := r.mask(s)
		return m, m != s
	case reflect.Bool:
		return false, fv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return 0, !fv.IsZero()
	case reflect.Slice:
		if fv.IsNil() {
			return nil, false
		}
		switch fv.Type().Elem().Kind() {
		case reflect.Uint8:
			// []byte encodes as a base64 string
			return maskText, true
		case reflect.String:
			ss := make([]string, fv.Len())
			for i := range ss {
				ss[i] = fv.Index(i).String()
			}
			return r.maskStrings(ss)
		}
		return []interface{}{}, fv.Len() > 0
	case reflect.Array:
		return []interface{}{}, fv.Len() > 0
	case reflect.Map:
		if fv.IsNil() {
			return nil, false
		}
		return map[string]interface{}{}, fv.Len() > 0
	case reflect.Struct:
		return map[string]interface{}{}, true
	}
	return nil, false
}

// maskEncoded masks a value with custom JSON encoding by the JSON type it
// actually produces.
func (r *Redactor) maskEncoded(v interface{}) (interface{}, bool) {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return maskText, true
	}
	switch b[0] {
	case '"':
		var s string
		if json.Unmarshal(b, &s) == nil {
			return r.mask(s), true
		}
		return maskText, true
	case 'n':
		return nil, false
	case 't', 'f':
		return false, true
	case '{':
		return map[string]interface{}{}, true
	case '[':
		return []interface{}{}, true
	}
	return 0, true
}

func hasMarshaler(t reflect.Type) bool {
	if t.Implements(jsonMarshalerType) || t.Implements(textMarshalerType) {
		return true
	}
	if t.Kind() != reflect.Ptr {
		pt := reflect.PointerTo(t)
		return pt.Implements(jsonMarshalerType) || pt.Implements(textMarshalerType)
	}
	return false
}

func (r *Redactor) redactValue(v interface{}, depth int) (interface{}, bool) {
	if v == nil || depth > r.maxDepth {
		return v, false
	}
	switch x := v.(type) {
	case string, bool, json.Number, []byte, []string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return v, false
	case map[string]interface{}:
		return r.redactMap(x, depth)
	case []interface{}:
		return r.redactSlice(x, depth)
	case map[string]string:
		return r.redactStringMap(x)
	case map[string][]string:
		return r.redactStringsMap(x)
	}
	return r.redactReflect(reflect.ValueOf(v), depth)
}

func (r *Redactor) redactMap(m map[string]interface{}, depth int) (interface{}, bool) {
	var changes map[string]interface{}
	for k, v := range m {
		var nv interface{}
		var changed bool
		if r.isSensitive(k) {
			nv, changed = r.maskValue(v)
		} else {
			nv, changed = r.redactValue(v, depth+1)
		}
		if changed {
			if changes == nil {
				changes = make(map[string]interface{})
			}
			changes[k] = nv
		}
	}
	if changes == nil {
		return m, false
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	for k, v := range changes {
		out[k] = v
	}
	return out, true
}

func (r *Redactor) redactSlice(s []interface{}, depth int) (interface{}, bool) {
	var out []interface{}
	for i, v := range s {
		nv, changed := r.redactValue(v, depth+1)
		if !changed {
			continue
		}
		if out == nil {
			out = make([]interface{}, len(s))
			copy(out, s)
		}
		out[i] = nv
	}
	if out == nil {
		return s, false
	}
	return out, true
}

func (r *Redactor) redactStringMap(m map[string]string) (interface{}, bool) {
	var out map[string]string
	for k, v := range m {
		if !r.isSensitive(k) {
			continue
		}
		mv := r.mask(v)
		if mv == v {
			continue
		}
		if out == nil {
			out = make(map[string]string, len(m))
			for k2, v2 := range m {
				out[k2] = v2
			}
		}
		out[k] = mv
	}
	if out == nil {
		return m, false
	}
	return out, true
}

func (r *Redactor) redactStringsMap(m map[string][]string) (interface{}, bool) {
	var out map[string][]string
	for k, v := range m {
		if !r.isSensitive(k) {
			continue
		}
		mv, changed := r.maskStrings(v)
		if !changed {
			continue
		}
		if out == nil {
			out = make(map[string][]string, len(m))
			for k2, v2 := range m {
				out[k2] = v2
			}
		}
		out[k] = mv.([]string)
	}
	if out == nil {
		return m, false
	}
	return out, true
}

var (
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

func (r *Redactor) isOpaque(t reflect.Type) bool {
	return !r.deepMarshaler && hasMarshaler(t)
}

// needsWalk reports whether values of t may contain sensitive keys. The
// answer depends only on the type for structs without dynamic parts, so it is
// computed once per type.
func (r *Redactor) needsWalk(t reflect.Type) bool {
	if v, ok := r.types.Load(t); ok {
		return v.(bool)
	}
	res := r.computeNeedsWalk(t, make(map[reflect.Type]bool))
	r.types.Store(t, res)
	return res
}

// computeNeedsWalk must only be cached for the root type: results for types
// inside a cycle are provisional until the whole cycle has been visited.
func (r *Redactor) computeNeedsWalk(t reflect.Type, visiting map[reflect.Type]bool) bool {
	if r.isOpaque(t) {
		return false
	}
	switch t.Kind() {
	case reflect.Ptr, reflect.Array:
		return r.computeNeedsWalk(t.Elem(), visiting)
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return false
		}
		return r.computeNeedsWalk(t.Elem(), visiting)
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return true
		}
		return r.computeNeedsWalk(t.Elem(), visiting)
	case reflect.Interface:
		return true
	case reflect.Struct:
		if visiting[t] {
			return false
		}
		visiting[t] = true
		for _, f := range cachedFields(t) {
			if !f.flatten && r.isSensitive(f.name) {
				return true
			}
			if r.computeNeedsWalk(t.Field(f.index).Type, visiting) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func isScalarKind(k reflect.Kind) bool {
	switch k {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128,
		reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	}
	return false
}

func (r *Redactor) redactReflect(rv reflect.Value, depth int) (interface{}, bool) {
	if !rv.IsValid() || depth > r.maxDepth || isScalarKind(rv.Kind()) {
		return nil, false
	}
	if !r.needsWalk(rv.Type()) {
		return nil, false
	}
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return nil, false
		}
		return r.redactReflect(rv.Elem(), depth)
	case reflect.Struct:
		return r.redactStruct(rv, depth+1)
	case reflect.Map:
		return r.redactReflectMap(rv, depth+1)
	case reflect.Slice, reflect.Array:
		return r.redactReflectSlice(rv, depth+1)
	}
	return nil, false
}

func (r *Redactor) redactStruct(rv reflect.Value, depth int) (interface{}, bool) {
	var changes map[string]interface{}
	r.scanStruct(rv, depth, &changes)
	if changes == nil {
		return nil, false
	}
	out := make(map[string]interface{})
	structToMap(rv, out)
	for k, v := range changes {
		out[k] = v
	}
	return out, true
}

func (r *Redactor) scanStruct(rv reflect.Value, depth int, changes *map[string]interface{}) {
	for _, f := range cachedFields(rv.Type()) {
		fv := rv.Field(f.index)
		if f.flatten {
			if fv.Kind() == reflect.Ptr {
				if fv.IsNil() {
					continue
				}
				fv = fv.Elem()
			}
			r.scanStruct(fv, depth, changes)
			continue
		}
		if !fv.CanInterface() || f.omitted(fv) {
			continue
		}
		var nv interface{}
		var changed bool
		if r.isSensitive(f.name) {
			nv, changed = r.maskReflect(fv)
		} else {
			nv, changed = r.redactReflect(fv, depth)
		}
		if changed {
			if *changes == nil {
				*changes = make(map[string]interface{})
			}
			(*changes)[f.name] = nv
		}
	}
}

func structToMap(rv reflect.Value, out map[string]interface{}) {
	for _, f := range cachedFields(rv.Type()) {
		fv := rv.Field(f.index)
		if f.flatten {
			if fv.Kind() == reflect.Ptr {
				if fv.IsNil() {
					continue
				}
				fv = fv.Elem()
			}
			structToMap(fv, out)
			continue
		}
		if !fv.CanInterface() || f.omitted(fv) {
			continue
		}
		out[f.name] = boxValue(fv)
	}
}

// boxValue keeps addressable values behind a pointer so that encoding/json
// still finds pointer receiver marshal methods, as it would on the original.
func boxValue(fv reflect.Value) interface{} {
	switch fv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice:
		return fv.Interface()
	}
	if fv.CanAddr() {
		return fv.Addr().Interface()
	}
	return fv.Interface()
}

func (r *Redactor) redactReflectMap(rv reflect.Value, depth int) (interface{}, bool) {
	if rv.IsNil() || rv.Len() == 0 {
		return nil, false
	}
	if rv.CanInterface() {
		switch x := rv.Interface().(type) {
		case map[string]interface{}:
			return r.redactMap(x, depth)
		case map[string]string:
			return r.redactStringMap(x)
		case map[string][]string:
			return r.redactStringsMap(x)
		}
	}
	strKey := rv.Type().Key().Kind() == reflect.String
	var changes map[string]interface{}
	iter := rv.MapRange()
	for iter.Next() {
		k, v := iter.Key(), iter.Value()
		var nv interface{}
		var changed bool
		if strKey && r.isSensitive(k.String()) {
			nv, changed = r.maskReflect(v)
		} else {
			nv, changed = r.redactReflect(v, depth)
		}
		if changed {
			if changes == nil {
				changes = make(map[string]interface{})
			}
			changes[mapKeyString(k)] = nv
		}
	}
	if changes == nil {
		return nil, false
	}
	out := make(map[string]interface{}, rv.Len())
	iter = rv.MapRange()
	for iter.Next() {
		out[mapKeyString(iter.Key())] = iter.Value().Interface()
	}
	for k, v := range changes {
		out[k] = v
	}
	return out, true
}

func mapKeyString(k reflect.Value) string {
	switch k.Kind() {
	case reflect.String:
		return k.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10)
	}
	return fmt.Sprint(k.Interface())
}

func (r *Redactor) redactReflectSlice(rv reflect.Value, depth int) (interface{}, bool) {
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return nil, false
	}
	var out []interface{}
	n := rv.Len()
	for i := 0; i < n; i++ {
		nv, changed := r.redactReflect(rv.Index(i), depth)
		if !changed {
			continue
		}
		if out == nil {
			out = make([]interface{}, n)
			for j := 0; j < n; j++ {
				out[j] = boxValue(rv.Index(j))
			}
		}
		out[i] = nv
	}
	if out == nil {
		return nil, false
	}
	return out, true
}

type fieldInfo struct {
	index     int
	name      string
	omitEmpty bool
	omitZero  bool
	flatten   bool
}

func (f *fieldInfo) omitted(v reflect.Value) bool {
	return (f.omitEmpty && isEmptyValue(v)) || (f.omitZero && v.IsZero())
}

var fieldsCache sync.Map // reflect.Type -> []fieldInfo

func cachedFields(t reflect.Type) []fieldInfo {
	if v, ok := fieldsCache.Load(t); ok {
		return v.([]fieldInfo)
	}
	fields := make([]fieldInfo, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				fields = append(fields, fieldInfo{index: i, flatten: true})
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fi := fieldInfo{index: i, name: name}
		for opts != "" {
			var opt string
			opt, opts, _ = strings.Cut(opts, ",")
			switch opt {
			case "omitempty":
				fi.omitEmpty = true
			case "omitzero":
				fi.omitZero = true
			}
		}
		fields = append(fields, fi)
	}
	v, _ := fieldsCache.LoadOrStore(t, fields)
	return v.([]fieldInfo)
}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Ptr:
		return v.IsZero()
	}
	return false
}

func (r *Redactor) isSensitive(key string) bool {
	sensitive, _ := r.classifyKey(key)
	return sensitive
}

// classifyKey walks key backwards through the trie, normalizing on the fly
// the same way normalizeKey does, so matching never allocates. A trailing
// plural "s" is retried without it, so "tokens" matches "token".
func (r *Redactor) classifyKey(key string) (sensitive, jsonKey bool) {
	i := skipSeparators(key, len(key)-1)
	if i < 0 {
		return false, false
	}
	sensitive, jsonKey = r.walkTrie(key, i)
	if !sensitive && lowerASCII(key[i]) == 's' {
		if j := skipSeparators(key, i-1); j >= 0 {
			sensitive, _ = r.walkTrie(key, j)
		}
	}
	return sensitive, jsonKey
}

func (r *Redactor) walkTrie(key string, i int) (sensitive, jsonKey bool) {
	n := r.trie
	for {
		if n = n.child(lowerASCII(key[i])); n == nil {
			return false, false
		}
		if i = skipSeparators(key, i-1); i < 0 {
			return n.sensitive, n.json
		}
		if n.sensitive && n.suffix {
			return true, false
		}
	}
}

func skipSeparators(key string, i int) int {
	for i >= 0 && (key[i] == '_' || key[i] == '-') {
		i--
	}
	return i
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// normalizeKey lowercases ASCII letters and drops '_' and '-', so that
// "id_card", "idCard" and "ID-Card" all become "idcard".
func normalizeKey(key string) string {
	buf := make([]byte, 0, len(key))
	for i := 0; i < len(key); i++ {
		if key[i] != '_' && key[i] != '-' {
			buf = append(buf, lowerASCII(key[i]))
		}
	}
	return string(buf)
}
