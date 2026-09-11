package tpl

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
)

// fakeserverFuncs returns the FuncMap of all fakeserver-owned functions
// from design §4.3, with osenvWhitelist controlling the osenv allowlist.
//
// Names align with the tplfunc roadmap (design §4.2/§4.3) so that if
// gookit/tplfunc ever implements them upstream we can drop ours.
func fakeserverFuncs(osenvWhitelist []string) template.FuncMap {
	allowed := osenvAllowlist(osenvWhitelist)

	return template.FuncMap{
		// === Identity / generic ===
		"uuid":     funcUUID,
		"shortid":  funcShortID,
		"incr":     funcIncr,
		"default":  funcDefault,
		"coalesce": funcCoalesce,

		// === Time ===
		"now":       funcNow,
		"timestamp": funcTimestamp,
		"addDate":   funcAddDate,

		// === Environment ===
		"env":       funcEnv, // Phase 3: always returns default
		"osenv":     makeOsenv(allowed),
		"expandEnv": makeExpandEnv(allowed),

		// === Random ===
		"randInt":    funcRandInt,
		"randFloat":  funcRandFloat,
		"randString": funcRandString,
		"randChoice": funcRandChoice,
		"shuffle":    funcShuffle,
		"weighted":   funcWeighted,

		// === Encoding ===
		"b64enc":     funcB64Enc,
		"b64dec":     funcB64Dec,
		"urlenc":     funcURLEnc,
		"urldec":     funcURLDec,
		"jsonEscape": funcJSONEscape,

		// === JSON ===
		"toJson":    funcToJSON,
		"fromJson":  funcFromJSON,
		"jsonValue": funcToJSON,
		"jsonPath":  funcJSONPath,

		// === String additions ===
		"title": funcTitle,
		"split": funcSplit,

		// === Control / debug ===
		"fail":  funcFail,
		"print": funcPrint,
	}
}

// ---- identity / generic ----

func funcUUID() string { return uuid.NewString() }

const base62Alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func funcShortID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = base62Alphabet[rand.Intn(len(base62Alphabet))]
	}
	return string(b)
}

var (
	incrMu   sync.Mutex
	incrPool = map[string]int64{}
)

func funcIncr(name string) string {
	incrMu.Lock()
	defer incrMu.Unlock()
	incrPool[name]++
	return strconv.FormatInt(incrPool[name], 10)
}

func funcDefault(fallback, v any) any {
	if isZero(v) {
		return fallback
	}
	return v
}

func funcCoalesce(vs ...any) any {
	for _, v := range vs {
		if !isZero(v) {
			return v
		}
	}
	return ""
}

func isZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case int:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	case bool:
		return !x
	}
	return false
}

// ---- time ----

func funcNow(layout ...string) any {
	t := time.Now()
	if len(layout) == 0 {
		return t
	}
	return t.Format(layout[0])
}

func funcTimestamp(unit ...string) string {
	t := time.Now()
	u := "s"
	if len(unit) > 0 {
		u = unit[0]
	}
	switch u {
	case "ms":
		return strconv.FormatInt(t.UnixMilli(), 10)
	case "us":
		return strconv.FormatInt(t.UnixMicro(), 10)
	case "ns":
		return strconv.FormatInt(t.UnixNano(), 10)
	default:
		return strconv.FormatInt(t.Unix(), 10)
	}
}

func funcAddDate(years, months, days int) time.Time {
	return time.Now().AddDate(years, months, days)
}

// ---- environment ----

// funcEnv returns the default value in Phase 3 (env file not yet
// available; design §4.3 says env reads .env which is empty before v0.2).
func funcEnv(args ...string) string {
	if len(args) >= 2 {
		return args[1]
	}
	return ""
}

func osenvAllowlist(list []string) map[string]struct{} {
	if len(list) == 0 {
		return nil // nil ≡ "allow all"
	}
	out := make(map[string]struct{}, len(list))
	for _, k := range list {
		out[k] = struct{}{}
	}
	return out
}

func makeOsenv(allowed map[string]struct{}) func(args ...string) string {
	return func(args ...string) string {
		if len(args) == 0 {
			return ""
		}
		key := args[0]
		def := ""
		if len(args) >= 2 {
			def = args[1]
		}
		if allowed != nil {
			if _, ok := allowed[key]; !ok {
				return def
			}
		}
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return def
	}
}

func makeExpandEnv(allowed map[string]struct{}) func(s string) string {
	osenv := makeOsenv(allowed)
	return func(s string) string {
		return os.Expand(s, func(k string) string { return osenv(k) })
	}
}

// ---- random ----

func funcRandInt(min, max int) int {
	if max < min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

func funcRandFloat(min, max float64) float64 {
	if max < min {
		return min
	}
	return min + rand.Float64()*(max-min)
}

func funcRandString(n int, charset ...string) string {
	cs := "alnum"
	if len(charset) > 0 {
		cs = charset[0]
	}
	var pool string
	switch cs {
	case "alpha":
		pool = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	case "hex":
		pool = "0123456789abcdef"
	case "base62":
		pool = base62Alphabet
	default: // "alnum"
		pool = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = pool[rand.Intn(len(pool))]
	}
	return string(b)
}

func funcRandChoice(args ...any) any {
	if len(args) == 0 {
		return ""
	}
	// 如果首参是 slice，从中选一项
	if len(args) == 1 {
		if slice, ok := args[0].([]any); ok && len(slice) > 0 {
			return slice[rand.Intn(len(slice))]
		}
		// 单个非 slice：直接返回
		return args[0]
	}
	return args[rand.Intn(len(args))]
}

func funcShuffle(list []any) []any {
	out := make([]any, len(list))
	copy(out, list)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// funcWeighted: 接受偶数个参数 (weight1, value1, weight2, value2, ...)
// 简化实现：仅支持 weight 为整数。
func funcWeighted(args ...any) any {
	if len(args)%2 != 0 || len(args) == 0 {
		return ""
	}
	totalWeight := 0
	for i := 0; i < len(args); i += 2 {
		if w, ok := args[i].(int); ok {
			totalWeight += w
		}
	}
	if totalWeight == 0 {
		return args[1]
	}
	pick := rand.Intn(totalWeight)
	cum := 0
	for i := 0; i < len(args); i += 2 {
		if w, ok := args[i].(int); ok {
			cum += w
			if pick < cum {
				return args[i+1]
			}
		}
	}
	return args[len(args)-1]
}

// ---- encoding ----

func funcB64Enc(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
func funcB64Dec(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}
func funcURLEnc(s string) string { return url.QueryEscape(s) }
func funcURLDec(s string) string {
	v, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return v
}

// funcJSONEscape returns the JSON string-escaped form of v WITHOUT the
// surrounding quotes — handy for injecting dynamic strings into JSON
// literal templates.
func funcJSONEscape(v any) string {
	b, err := json.Marshal(fmt.Sprint(v))
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ---- JSON ----

func funcToJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func funcFromJSON(s string) any {
	var out any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// funcJSONPath traverses a nested map/slice by dotted/indexed key path,
// e.g. "a.b[0].c". Missing path returns empty string.
func funcJSONPath(obj any, path string) any {
	cur := obj
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '.' })
	for _, raw := range parts {
		// strip array index suffix like "key[2]"
		key := raw
		idx := -1
		if lb := strings.Index(raw, "["); lb >= 0 && strings.HasSuffix(raw, "]") {
			key = raw[:lb]
			n, err := strconv.Atoi(raw[lb+1 : len(raw)-1])
			if err == nil {
				idx = n
			}
		}
		if key != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return ""
			}
			cur, ok = m[key]
			if !ok {
				return ""
			}
		}
		if idx >= 0 {
			arr, ok := cur.([]any)
			if !ok || idx >= len(arr) {
				return ""
			}
			cur = arr[idx]
		}
	}
	return cur
}

// ---- string additions ----

// funcTitle: ASCII-only title-cases each whitespace-separated word.
// stdlib's strings.Title is deprecated; cases.Title pulls in golang.org/x/text.
// design §4.3 just wants "hello world" → "Hello World" for common ASCII use.
func funcTitle(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func funcSplit(sep, s string) []string {
	return strings.Split(s, sep)
}

// ---- control / debug ----

// funcFail forces the current template render to error, bubbling up to a
// 500 response (design §4.5).
func funcFail(msg string) (string, error) {
	return "", fmt.Errorf("template fail: %s", msg)
}

func funcPrint(v any) string {
	fmt.Fprintln(os.Stderr, "[tpl print]", v)
	return ""
}
