package logger

import (
	"encoding/json"
	"strings"
	"testing"
)

func serverLogExtra() map[string]interface{} {
	return map[string]interface{}{
		"method":         "/config.Service/Get",
		"duration":       "12.345ms",
		"codeString":     "OK",
		"code":           0,
		"peer":           "10.0.3.17:52144",
		"caller_service": "iamsvc",
		"name":           "configsvc",
	}
}

func benchBody(size int, sensitive bool) string {
	var b strings.Builder
	b.WriteString(`{"app_id":"demo","items":[`)
	for i := 0; b.Len() < size; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"sku":"A-10086","count":3,"remark":"normal order item"}`)
	}
	b.WriteString(`]`)
	if sensitive {
		b.WriteString(`,"password":"hunter2hunter2"`)
	}
	b.WriteString(`}`)
	return b.String()
}

func BenchmarkScalarExtra(b *testing.B) {
	r := NewRedactor()
	extra := serverLogExtra()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.RedactExtra(extra)
	}
}

func BenchmarkProtoNoHit(b *testing.B) {
	r := NewRedactor()
	extra := map[string]interface{}{"req": &pbGetReq{Id: 1, AppId: "demo", Status: 1, Inner: &pbGetReq{Id: 2}}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r.RedactExtra(extra)
	}
}

func BenchmarkProtoHit(b *testing.B) {
	r := NewRedactor()
	req := &pbLoginReq{
		Id:     7,
		Phone:  "13812345678",
		Status: 1,
		Inner:  &pbInner{AccessToken: "abcdefghijklmnop", Region: "cn"},
		Tags:   []string{"a", "b"},
		Login:  &PbLoginReq_Password{Password: "p@ssw0rd!"},
	}
	extra := map[string]interface{}{"req": req}

	b.Run("redact", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r.RedactExtra(extra)
		}
	})
	// baseline: what zap already pays to encode the same message
	b.Run("json_marshal_baseline", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = json.Marshal(extra)
		}
	})
}

func BenchmarkHTTPBody(b *testing.B) {
	r := NewRedactor(WithMaxStringLen(0))
	for _, c := range []struct {
		name      string
		size      int
		sensitive bool
	}{
		{"1KB_miss", 1 << 10, false},
		{"10KB_miss", 10 << 10, false},
		{"1KB_hit", 1 << 10, true},
		{"10KB_hit", 10 << 10, true},
	} {
		extra := map[string]interface{}{"body": benchBody(c.size, c.sensitive)}
		b.Run(c.name, func(b *testing.B) {
			b.SetBytes(int64(len(extra["body"].(string))))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				r.RedactExtra(extra)
			}
		})
	}
}
