package jsonedit

import (
	"strings"
	"testing"
)

func TestParseRejects(t *testing.T) {
	// Go's decoder words truncation differently across releases ("EOF" up to 1.26,
	// "unexpected end of JSON input" in 1.27); any of these means the input stopped early.
	for _, tc := range []struct{ name, in, want string }{
		{"not an object", `[1]`, "not a JSON object"},
		{"empty", ``, "EOF|end of JSON input"},
		{"truncated", `{"a":1`, "EOF|end of JSON input"},
		{"duplicate key", `{"a":1,"a":2}`, `key "a" appears twice`},
		{"trailing data", `{"a":1} {}`, "unexpected data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.in))
			if err == nil || !containsAny(err.Error(), strings.Split(tc.want, "|")) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestFormatKeepsOrderAndWrittenValues(t *testing.T) {
	in := `{ "z": ["~/a",  "café", "a&b"],"a":{"k":1.50,"b":null},
	"m":"x"}`
	o, err := Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "z": [
    "~/a",
    "café",
    "a&b"
  ],
  "a": {
    "k": 1.50,
    "b": null
  },
  "m": "x"
}
`
	if got := string(o.Format()); got != want {
		t.Errorf("Format =\n%s\nwant\n%s", got, want)
	}
}

func TestSetPlacesNewKeysInOrder(t *testing.T) {
	order := []string{"a", "b", "c", "d"}
	for _, tc := range []struct{ name, in, key, want string }{
		{"between", `{"a":1,"d":4}`, "b", `{"a":1,"b":0,"d":4}`},
		{"last", `{"a":1,"b":2}`, "c", `{"a":1,"b":2,"c":0}`},
		{"first", `{"b":2,"c":3}`, "a", `{"a":0,"b":2,"c":3}`},
		{"unordered key goes last", `{"a":1}`, "zz", `{"a":1,"zz":0}`},
		{"existing keeps place", `{"c":1,"a":2}`, "a", `{"c":1,"a":0}`},
		{"before the first later key", `{"d":1,"a":2}`, "b", `{"b":0,"d":1,"a":2}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, err := Parse([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			o.Set(tc.key, []byte("0"), order)
			if got := string(o.Marshal()); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	o, _ := Parse([]byte(`{"a":1,"b":2,"c":3}`))
	o.Delete("b")
	o.Delete("missing")
	if got := string(o.Marshal()); got != `{"a":1,"c":3}` {
		t.Errorf("got %s", got)
	}
}

func TestChild(t *testing.T) {
	o, _ := Parse([]byte(`{"a":{"x":1},"s":"str"}`))
	c, err := o.Child("a")
	if err != nil || string(c.Marshal()) != `{"x":1}` {
		t.Errorf("Child(a) = %s, %v", c.Marshal(), err)
	}
	if c, err := o.Child("none"); err != nil || string(c.Marshal()) != `{}` {
		t.Errorf("Child(none) = %s, %v", c.Marshal(), err)
	}
	if _, err := o.Child("s"); err == nil {
		t.Error("Child of a string: want an error")
	}
}

func TestStringKeepsHTMLCharacters(t *testing.T) {
	if got := string(String(`a&b<c>"d"`)); got != `"a&b<c>\"d\""` {
		t.Errorf("String = %s", got)
	}
}

func TestArray(t *testing.T) {
	a, err := ParseArray([]byte(`["~/a", "café" , "/b"]`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(a.WithoutStrings([]string{"café"}).Marshal()); got != `["~/a","/b"]` {
		t.Errorf("WithoutStrings(café) = %s, want the decoded value matched", got)
	}
	if got := string(a.WithoutStrings([]string{"nope"}).Marshal()); got != `["~/a","café","/b"]` {
		t.Errorf("WithoutStrings(nope) = %s", got)
	}
	if got := string(a.Marshal()); got != `["~/a","café","/b"]` {
		t.Errorf("Marshal = %s, want the written forms", got)
	}
	if got := string(a.WithoutStrings([]string{"~/a", "café", "/b"}).Marshal()); got != `[]` {
		t.Errorf("empty = %s", got)
	}
	if a, err := ParseArray(nil); err != nil || len(a) != 0 {
		t.Errorf("ParseArray(nil) = %v, %v", a, err)
	}
	if _, err := ParseArray([]byte(`{}`)); err == nil {
		t.Error("ParseArray of an object: want an error")
	}
}
