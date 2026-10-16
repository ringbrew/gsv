package logger

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The types below mimic the shape of protoc-gen-go output.

type pbStatus int32

func (s pbStatus) String() string { return [...]string{"Invalid", "Ok"}[s] }

type pbInner struct {
	state     struct{}
	sizeCache int32

	AccessToken string `protobuf:"bytes,1,opt,name=access_token,proto3" json:"access_token,omitempty"`
	Region      string `protobuf:"bytes,2,opt,name=region,proto3" json:"region,omitempty"`
}

type isPbLoginReq_Login interface{ isPbLoginReq_Login() }

type PbLoginReq_Password struct {
	Password string `protobuf:"bytes,10,opt,name=password,proto3,oneof"`
}

func (*PbLoginReq_Password) isPbLoginReq_Login() {}

type pbLoginReq struct {
	state         struct{}
	sizeCache     int32
	unknownFields []byte

	Id     int64              `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	Name   string             `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Phone  string             `protobuf:"bytes,3,opt,name=phone,proto3" json:"phone,omitempty"`
	Status pbStatus           `protobuf:"varint,4,opt,name=status,proto3,enum=x.Status" json:"status,omitempty"`
	Inner  *pbInner           `protobuf:"bytes,5,opt,name=inner,proto3" json:"inner,omitempty"`
	Tags   []string           `protobuf:"bytes,6,rep,name=tags,proto3" json:"tags,omitempty"`
	Login  isPbLoginReq_Login `protobuf_oneof:"login"`
}

type pbGetReq struct {
	state     struct{}
	sizeCache int32

	Id     int64     `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	AppId  string    `protobuf:"bytes,2,opt,name=app_id,proto3" json:"app_id,omitempty"`
	Status pbStatus  `protobuf:"varint,3,opt,name=status,proto3,enum=x.Status" json:"status,omitempty"`
	Inner  *pbGetReq `protobuf:"bytes,4,opt,name=inner,proto3" json:"inner,omitempty"`
}

type treeNode struct {
	Name     string      `json:"name"`
	Password string      `json:"password,omitempty"`
	Children []*treeNode `json:"children,omitempty"`
}

type withTime struct {
	CreatedAt time.Time `json:"created_at"`
	Secret    string    `json:"secret"`
}

type ptrMarshaler struct{ v int }

func (p *ptrMarshaler) MarshalJSON() ([]byte, error) { return []byte(`"custom"`), nil }

type withPtrMarshaler struct {
	M     ptrMarshaler `json:"m"`
	Token string       `json:"token"`
}

type Base struct {
	Email string `json:"email"`
	Owner string `json:"owner"`
}

type embedded struct {
	Base
	Title string `json:"title"`
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func sameJSON(t *testing.T, got interface{}, want string) {
	t.Helper()
	var g, w interface{}
	if err := json.Unmarshal([]byte(mustJSON(t, got)), &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("got  %s\nwant %s", mustJSON(t, got), want)
	}
}

func samePointer(a, b interface{}) bool {
	return reflect.ValueOf(a).UnsafePointer() == reflect.ValueOf(b).UnsafePointer()
}

func TestDefaultMask(t *testing.T) {
	cases := map[string]string{
		"":                   "",
		"123456":             "***",
		"中文名":                "***",
		"abcdefg":            "a*****g",
		"abcdefghij":         "a*****j",
		"13812345678":        "138******78",
		"110101199003071234": "110******34",
		"张三丰的身份证号码很长":        "张三丰******很长",
	}
	for in, want := range cases {
		got := DefaultMask(in)
		if got != want {
			t.Errorf("DefaultMask(%q) = %q, want %q", in, got, want)
		}
		if again := DefaultMask(got); again != got {
			t.Errorf("DefaultMask not idempotent on %q: %q -> %q", in, got, again)
		}
	}
}

func TestKeyMatching(t *testing.T) {
	r := NewRedactor()
	hits := []string{
		"password", "Password", "PASSWORD", "user_password", "userPassword",
		"id_card", "idCard", "ID-Card", "IDCard",
		"access_token", "nextPageToken", "X-Auth-Token", "Authorization", "X-Api-Key",
		"sign", "otp", "contact_email", "telephone", "_password_", strings.Repeat("a", 70) + "password",
		"tokens", "Phones", "user_emails", "PASSWORDS", "signs",
	}
	misses := []string{
		"tokenCount", "signup", "design", "assign", "method", "duration",
		"code", "codeString", "name", "app_id", "", "__", "body", "a.password.b",
		"status", "address", "ids", "codes", "s", "sms",
	}
	for _, k := range hits {
		if !r.isSensitive(k) {
			t.Errorf("%q should be sensitive", k)
		}
	}
	for _, k := range misses {
		if r.isSensitive(k) {
			t.Errorf("%q should not be sensitive", k)
		}
	}
	for k, want := range map[string]bool{"body": true, "Body": true, "req": true, "RESP": true, "req_body": false, "bodies": false} {
		if _, got := r.classifyKey(k); got != want {
			t.Errorf("classifyKey(%q) json = %v, want %v", k, got, want)
		}
	}
}

func TestRedactorOptions(t *testing.T) {
	r := NewRedactor(WithoutSensitiveKeys("email"), WithSensitiveKeys("wechat_openid"))
	if r.isSensitive("email") {
		t.Error("email should have been removed")
	}
	if r.isSensitive("contact_email") {
		t.Error("removed key should not match as suffix either")
	}
	if !r.isSensitive("wechatOpenId") || !r.isSensitive("user_wechat_openid") {
		t.Error("added key should match exactly and as suffix")
	}
}

func TestRedactExtraNoHitKeepsMap(t *testing.T) {
	r := NewRedactor()
	extra := map[string]interface{}{
		"method":   "/config.Service/Get",
		"duration": "12.345ms",
		"code":     0,
		"nested":   map[string]interface{}{"app_id": "demo", "list": []interface{}{1, "x"}},
	}
	got := r.RedactExtra(extra)
	if !samePointer(got, extra) {
		t.Fatal("unchanged extra should be returned as is")
	}
}

func TestRedactExtraNestedHit(t *testing.T) {
	r := NewRedactor()
	nested := map[string]interface{}{
		"app_id": "demo",
		"users":  []interface{}{map[string]interface{}{"mobile": "13812345678", "age": 3}},
	}
	extra := map[string]interface{}{
		"method":   "POST",
		"password": "hunter2hunter2",
		"apiKey":   12345,
		"data":     nested,
	}
	got := r.RedactExtra(extra)
	sameJSON(t, got, `{"method":"POST","password":"hun******r2","apiKey":0,"data":{"app_id":"demo","users":[{"mobile":"138******78","age":3}]}}`)

	if extra["password"] != "hunter2hunter2" {
		t.Error("original extra must not be modified")
	}
	if nested["users"].([]interface{})[0].(map[string]interface{})["mobile"] != "13812345678" {
		t.Error("original nested value must not be modified")
	}
}

type phoneHolder struct {
	Phone   int64             `json:"phone"`
	Mobile  bool              `json:"is_mobile"`
	Session map[string]string `json:"session"`
	Tokens  []int             `json:"tokens"`
	Secret  []byte            `json:"secret"`
	Email   *string           `json:"email"`
}

// A sensitive field must keep its JSON type after masking, otherwise
// Elasticsearch rejects the document when the field is already mapped.
func TestMaskKeepsJSONType(t *testing.T) {
	r := NewRedactor()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	extra := map[string]interface{}{
		"phone":     13812345678,
		"is_mobile": true,
		"session":   map[string]interface{}{"id": "x"},
		"tokens":    []interface{}{"a", 1},
		"secret":    []byte("raw"),
		"sign":      nil,
		"otp":       0,
		"sessionid": ts,
		"req":       &phoneHolder{Phone: 13812345678, Mobile: true, Session: map[string]string{"a": "b"}, Tokens: []int{1}, Secret: []byte("x")},
		"body":      `{"phone":13812345678,"is_mobile":true,"token":{"a":1},"tokens":[1,2]}`,
	}
	got := r.RedactExtra(extra)
	sameJSON(t, got, `{
		"phone":0,"is_mobile":false,"session":{},"tokens":[],"secret":"***","sign":null,"otp":0,
		"sessionid":"202******5Z",
		"req":{"phone":0,"is_mobile":false,"session":{},"tokens":[],"secret":"***","email":null},
		"body":"{\"is_mobile\":false,\"phone\":0,\"token\":{},\"tokens\":[]}"
	}`)
	if mustJSON(t, r.RedactExtra(got)) != mustJSON(t, got) {
		t.Error("type preserving mask must be idempotent")
	}
}

func TestRedactIdempotent(t *testing.T) {
	r := NewRedactor()
	extra := map[string]interface{}{
		"password": "hunter2hunter2",
		"pin_code": 1234,
		"body":     `{"login":{"password":"abc","note":"` + strings.Repeat("x", 5000) + `"}}`,
		"req":      &pbLoginReq{Phone: "13812345678", Login: &PbLoginReq_Password{Password: "p@ss"}},
	}
	once := r.RedactExtra(extra)
	twice := r.RedactExtra(once)
	if mustJSON(t, once) != mustJSON(t, twice) {
		t.Fatalf("not idempotent:\nonce  %s\ntwice %s", mustJSON(t, once), mustJSON(t, twice))
	}
}

func TestJSONTextBody(t *testing.T) {
	r := NewRedactor()

	clean := `{"app_id":"demo","admin_id":1}`
	if got := r.RedactExtra(map[string]interface{}{"body": clean})["body"]; got != clean {
		t.Errorf("clean body changed: %v", got)
	}

	hit := `{"user":{"phone":"13812345678","id":9007199254740993},"items":[{"token":"abcdefghijklmn"}],"html":"<b>&</b>"}`
	got := r.RedactExtra(map[string]interface{}{"body": hit})["body"].(string)
	want := `{"html":"<b>&</b>","items":[{"token":"abc******mn"}],"user":{"id":9007199254740993,"phone":"138******78"}}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	valueOnly := `{"note":"password","msg":"token"}`
	if got := r.RedactExtra(map[string]interface{}{"body": valueOnly})["body"]; got != valueOnly {
		t.Errorf("sensitive words as values must not trigger rewrite: %v", got)
	}

	broken := `{"password":"abc"`
	if got := r.RedactExtra(map[string]interface{}{"body": broken})["body"]; got != unparsableText {
		t.Errorf("broken JSON with sensitive key should be fully redacted, got %v", got)
	}

	notJSON := "plain text password=1"
	if got := r.RedactExtra(map[string]interface{}{"body": notJSON})["body"]; got != notJSON {
		t.Errorf("non JSON body changed: %v", got)
	}

	if got := r.RedactExtra(map[string]interface{}{"other": hit})["other"]; got != hit {
		t.Error("only jsonKeys are parsed")
	}
}

func TestTruncate(t *testing.T) {
	r := NewRedactor(WithMaxStringLen(20))
	s := strings.Repeat("中", 10)
	got := r.RedactExtra(map[string]interface{}{"msg2": s})["msg2"].(string)
	if len(got) > 20 || !strings.HasSuffix(got, truncatedSuffix) {
		t.Errorf("bad truncation: %q (%d bytes)", got, len(got))
	}
	if !strings.HasPrefix(got, "中") || !utf8Valid(got) {
		t.Errorf("truncation must keep valid UTF-8: %q", got)
	}
	if again := r.RedactExtra(map[string]interface{}{"msg2": got})["msg2"]; again != got {
		t.Error("truncation must be idempotent")
	}

	r = NewRedactor(WithMaxStringLen(0))
	long := strings.Repeat("x", 10000)
	if got := r.RedactExtra(map[string]interface{}{"x": long})["x"]; got != long {
		t.Error("WithMaxStringLen(0) should disable truncation")
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "\uFFFD") == s }

func TestReflectNoHitKeepsValue(t *testing.T) {
	r := NewRedactor()
	req := &pbGetReq{Id: 1, AppId: "demo", Status: 1, Inner: &pbGetReq{Id: 2}}
	extra := map[string]interface{}{"req": req}
	got := r.RedactExtra(extra)
	if !samePointer(got, extra) || got["req"] != req {
		t.Fatal("struct without sensitive fields must be returned as is")
	}
	if r.needsWalk(reflect.TypeOf(req)) {
		t.Error("pbGetReq should be cached as not needing a walk")
	}
}

func TestReflectHit(t *testing.T) {
	r := NewRedactor()
	req := &pbLoginReq{
		Id:     7,
		Phone:  "13812345678",
		Status: 1,
		Inner:  &pbInner{AccessToken: "abcdefghijklmnop", Region: "cn"},
		Login:  &PbLoginReq_Password{Password: "p@ssw0rd!"},
	}
	before := mustJSON(t, req)

	got := r.RedactValue(req)
	// enum stays a number, empty Name and Tags stay omitted, oneof keeps its wrapper
	sameJSON(t, got, `{"id":7,"phone":"138******78","status":1,"inner":{"access_token":"abc******op","region":"cn"},"Login":{"Password":"p*****!"}}`)

	if mustJSON(t, req) != before {
		t.Error("original message must not be modified")
	}
}

func TestReflectRecursiveType(t *testing.T) {
	r := NewRedactor()
	root := &treeNode{Name: "root", Children: []*treeNode{{Name: "a", Children: []*treeNode{{Name: "b", Password: "secret123"}}}}}
	sameJSON(t, r.RedactValue(root), `{"name":"root","children":[{"name":"a","children":[{"name":"b","password":"s*****3"}]}]}`)

	clean := &treeNode{Name: "x"}
	if r.RedactValue(clean) != clean {
		t.Error("recursive type without values should be returned as is")
	}
}

func TestReflectOpaqueAndPointerMarshaler(t *testing.T) {
	r := NewRedactor()
	ts := time.Date(2026, 9, 4, 16, 46, 37, 1, time.FixedZone("CST", 8*3600))
	sameJSON(t, r.RedactValue(&withTime{CreatedAt: ts, Secret: "abcdefgh"}),
		`{"created_at":"2026-09-04T16:46:37.000000001+08:00","secret":"a*****h"}`)

	sameJSON(t, r.RedactValue(&withPtrMarshaler{Token: "abcdefgh"}), `{"m":"custom","token":"a*****h"}`)
}

func TestReflectEmbedded(t *testing.T) {
	r := NewRedactor()
	v := &embedded{Base: Base{Email: "foo@bar.com", Owner: "me"}, Title: "t"}
	sameJSON(t, r.RedactValue(v), `{"email":"foo******om","owner":"me","title":"t"}`)
}

func TestRedactQuery(t *testing.T) {
	r := NewRedactor()
	cases := map[string]string{
		"":                                   "",
		"page=1&size=20":                     "page=1&size=20",
		"page=1&access_token=abcdefghijklmn": "page=1&access_token=abc******mn",
		"&token=abcdefgh;x=1":                "&token=a*****h;x=1",
		"user%5Fpassword=abcdefgh&flag":      "user%5Fpassword=a*****h&flag",
		"password":                           "password",
	}
	for in, want := range cases {
		if got := r.RedactQuery(in); got != want {
			t.Errorf("RedactQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGlobalRedactorAndLazyExtra(t *testing.T) {
	entry := (&LogEntry{}).WithExtra("password", "hunter2hunter2")
	if entry.RedactedExtra()["password"] != "hun******r2" {
		t.Error("global redactor should be on by default")
	}

	prev := GetRedactor()
	SetRedactor(nil)
	defer SetRedactor(prev)
	if entry.RedactedExtra()["password"] != "hunter2hunter2" {
		t.Error("SetRedactor(nil) should disable redaction")
	}
}

func TestPackageLevelRedactsForLegacyLogger(t *testing.T) {
	prev := l
	defer func() { l = prev }()

	rec := &recordLogger{}
	l = rec
	rec.SetLevel(LevelInfo)

	Info(NewEntry().WithExtra("token", "abcdefghijklmn"))
	if rec.last["token"] != "abc******mn" {
		t.Errorf("package level Info should redact before calling Logger, got %v", rec.last["token"])
	}

	rec.last = nil
	Debug(NewEntry().WithExtra("token", "abcdefghijklmn"))
	if rec.last["token"] != "abcdefghijklmn" {
		t.Error("filtered level should skip redaction work")
	}
}

// recordLogger imitates a Logger built before redaction existed: it reads
// entry.Extra directly.
type recordLogger struct {
	MockLogger
	last map[string]interface{}
}

func (r *recordLogger) Debug(e *LogEntry) { r.last = e.Extra }
func (r *recordLogger) Info(e *LogEntry)  { r.last = e.Extra }
