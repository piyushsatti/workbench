// Package jsonedit changes a JSON object without disturbing what the user
// wrote: key order is kept and values nobody touches are carried through as the
// exact bytes they had.
package jsonedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Object struct {
	keys []string
	vals map[string]json.RawMessage
}

func New() *Object {
	return &Object{vals: map[string]json.RawMessage{}}
}

// Parse reads one JSON object. A repeated key is an error, because keeping the
// file's meaning through an edit would need a choice between the copies.
func Parse(data []byte) (*Object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
	o := New()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key := keyTok.(string)
		if _, dup := o.vals[key]; dup {
			return nil, fmt.Errorf("key %q appears twice", key)
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		o.keys = append(o.keys, key)
		o.vals[key] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the top-level object")
	}
	return o, nil
}

func (o *Object) Get(key string) (json.RawMessage, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// Child returns the object under key, or an empty one when the key is absent.
func (o *Object) Child(key string) (*Object, error) {
	v, ok := o.vals[key]
	if !ok {
		return New(), nil
	}
	c, err := Parse(v)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return c, nil
}

// Set replaces the value under key. A new key goes before the first existing
// key that comes later in order, or last when none does, so files keep one
// stable layout.
func (o *Object) Set(key string, v json.RawMessage, order []string) {
	if _, ok := o.vals[key]; ok {
		o.vals[key] = v
		return
	}
	rank := func(k string) int {
		for i, n := range order {
			if n == k {
				return i
			}
		}
		return len(order)
	}
	at := len(o.keys)
	for i, k := range o.keys {
		if rank(k) > rank(key) {
			at = i
			break
		}
	}
	o.keys = append(o.keys, "")
	copy(o.keys[at+1:], o.keys[at:])
	o.keys[at] = key
	o.vals[key] = v
}

func (o *Object) Delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

func (o *Object) Marshal() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(String(k))
		b.WriteByte(':')
		b.Write(o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes()
}

// Format is the object indented by two spaces with a trailing newline.
func (o *Object) Format() []byte {
	var out bytes.Buffer
	// Marshal assembles valid JSON, so Indent cannot fail.
	_ = json.Indent(&out, o.Marshal(), "", "  ")
	out.WriteByte('\n')
	return out.Bytes()
}

// String encodes s without the HTML escaping json.Marshal applies.
func String(s string) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return bytes.TrimRight(b.Bytes(), "\n")
}

// Array holds the elements of a JSON array as written.
type Array []json.RawMessage

// ParseArray reads a JSON array; an absent value (nil) is an empty array.
func ParseArray(v json.RawMessage) (Array, error) {
	if v == nil {
		return nil, nil
	}
	var a Array
	if err := json.Unmarshal(v, &a); err != nil {
		return nil, err
	}
	return a, nil
}

func (a Array) Marshal() json.RawMessage {
	parts := make([][]byte, len(a))
	for i, e := range a {
		parts[i] = e
	}
	return append(append([]byte{'['}, bytes.Join(parts, []byte{','})...), ']')
}

// Index returns the position of the first element that decodes to the string
// s, or -1.
func (a Array) Index(s string) int {
	for i, e := range a {
		var got string
		if json.Unmarshal(e, &got) == nil && got == s {
			return i
		}
	}
	return -1
}

func (a Array) Without(i int) Array {
	return append(append(Array{}, a[:i]...), a[i+1:]...)
}
