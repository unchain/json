// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package json

import (
	"bytes"
	"encoding"
	"fmt"
	"log"
	"math"
	"reflect"
	"strconv"
	"testing"
	"unicode"

	"github.com/e-nikolov/json/bson"
)

func TestEncodeRenamedByteSliceBSON(t *testing.T) {
	s := renamedByteSlice("abc")
	result, err := bson.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	expect := `"YWJj"`
	if string(result) != expect {
		t.Errorf(" got %s want %s", result, expect)
	}
	r := renamedRenamedByteSlice("abc")
	result, err = bson.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != expect {
		t.Errorf(" got %s want %s", result, expect)
	}
}

func TestSamePointerNoCycleBSON(t *testing.T) {
	if _, err := bson.Marshal(samePointerNoCycle); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSliceNoCycleBSON(t *testing.T) {
	if _, err := bson.Marshal(sliceNoCycle); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsupportedValuesBSON(t *testing.T) {
	for _, v := range unsupportedValues {
		if _, err := bson.Marshal(v); err != nil {
			if _, ok := err.(*UnsupportedValueError); !ok {
				t.Errorf("for %v, got %T want UnsupportedValueError", v, err)
			}
		} else {
			t.Errorf("for %v, expected error", v)
		}
	}
}

// Issue 43207
func TestMarshalTextFloatMapBSON(t *testing.T) {
	m := map[textfloat]string{
		textfloat(math.NaN()): "1",
		textfloat(math.NaN()): "1",
	}
	got, err := bson.Marshal(m)
	if err != nil {
		t.Errorf("Marshal() error: %v", err)
	}
	want := `{"TF:NaN":"1","TF:NaN":"1"}`
	if string(got) != want {
		t.Errorf("Marshal() = %s, want %s", got, want)
	}
}

func TestRefValMarshalBSON(t *testing.T) {
	var s = struct {
		R0 Ref
		R1 *Ref
		R2 RefText
		R3 *RefText
		V0 Val
		V1 *Val
		V2 ValText
		V3 *ValText
	}{
		R0: 12,
		R1: new(Ref),
		R2: 14,
		R3: new(RefText),
		V0: 13,
		V1: new(Val),
		V2: 15,
		V3: new(ValText),
	}
	const want = `{"R0":"ref","R1":"ref","R2":"\"ref\"","R3":"\"ref\"","V0":"val","V1":"val","V2":"\"val\"","V3":"\"val\""}`
	b, err := bson.Marshal(&s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(b); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarshalerEscapingBSON(t *testing.T) {
	var c C
	want := `"\u003c\u0026\u003e"`
	b, err := bson.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal(c): %v", err)
	}
	if got := string(b); got != want {
		t.Errorf("Marshal(c) = %#q, want %#q", got, want)
	}

	var ct CText
	want = `"\"\u003c\u0026\u003e\""`
	b, err = bson.Marshal(ct)
	if err != nil {
		t.Fatalf("Marshal(ct): %v", err)
	}
	if got := string(b); got != want {
		t.Errorf("Marshal(ct) = %#q, want %#q", got, want)
	}
}

func TestAnonymousFieldsBSON(t *testing.T) {
	tests := []struct {
		label     string             // Test name
		makeInput func() interface{} // Function to create input value
		want      string             // Expected JSON output
	}{{
		// Both S1 and S2 have a field named X. From the perspective of S,
		// it is ambiguous which one X refers to.
		// This should not serialize either field.
		label: "AmbiguousField",
		makeInput: func() interface{} {
			type (
				S1 struct{ x, X int }
				S2 struct{ x, X int }
				S  struct {
					S1
					S2
				}
			)
			return S{S1{1, 2}, S2{3, 4}}
		},
		want: `{}`,
	}, {
		label: "DominantField",
		// Both S1 and S2 have a field named X, but since S has an X field as
		// well, it takes precedence over S1.X and S2.X.
		makeInput: func() interface{} {
			type (
				S1 struct{ x, X int }
				S2 struct{ x, X int }
				S  struct {
					S1
					S2
					x, X int
				}
			)
			return S{S1{1, 2}, S2{3, 4}, 5, 6}
		},
		want: `{"X":6}`,
	}, {
		// Unexported embedded field of non-struct type should not be serialized.
		label: "UnexportedEmbeddedInt",
		makeInput: func() interface{} {
			type (
				myInt int
				S     struct{ myInt }
			)
			return S{5}
		},
		want: `{}`,
	}, {
		// Exported embedded field of non-struct type should be serialized.
		label: "ExportedEmbeddedInt",
		makeInput: func() interface{} {
			type (
				MyInt int
				S     struct{ MyInt }
			)
			return S{5}
		},
		want: `{"MyInt":5}`,
	}, {
		// Unexported embedded field of pointer to non-struct type
		// should not be serialized.
		label: "UnexportedEmbeddedIntPointer",
		makeInput: func() interface{} {
			type (
				myInt int
				S     struct{ *myInt }
			)
			s := S{new(myInt)}
			*s.myInt = 5
			return s
		},
		want: `{}`,
	}, {
		// Exported embedded field of pointer to non-struct type
		// should be serialized.
		label: "ExportedEmbeddedIntPointer",
		makeInput: func() interface{} {
			type (
				MyInt int
				S     struct{ *MyInt }
			)
			s := S{new(MyInt)}
			*s.MyInt = 5
			return s
		},
		want: `{"MyInt":5}`,
	}, {
		// Exported fields of embedded structs should have their
		// exported fields be serialized regardless of whether the struct types
		// themselves are exported.
		label: "EmbeddedStruct",
		makeInput: func() interface{} {
			type (
				s1 struct{ x, X int }
				S2 struct{ y, Y int }
				S  struct {
					s1
					S2
				}
			)
			return S{s1{1, 2}, S2{3, 4}}
		},
		want: `{"X":2,"Y":4}`,
	}, {
		// Exported fields of pointers to embedded structs should have their
		// exported fields be serialized regardless of whether the struct types
		// themselves are exported.
		label: "EmbeddedStructPointer",
		makeInput: func() interface{} {
			type (
				s1 struct{ x, X int }
				S2 struct{ y, Y int }
				S  struct {
					*s1
					*S2
				}
			)
			return S{&s1{1, 2}, &S2{3, 4}}
		},
		want: `{"X":2,"Y":4}`,
	}, {
		// Exported fields on embedded unexported structs at multiple levels
		// of nesting should still be serialized.
		label: "NestedStructAndInts",
		makeInput: func() interface{} {
			type (
				MyInt1 int
				MyInt2 int
				myInt  int
				s2     struct {
					MyInt2
					myInt
				}
				s1 struct {
					MyInt1
					myInt
					s2
				}
				S struct {
					s1
					myInt
				}
			)
			return S{s1{1, 2, s2{3, 4}}, 6}
		},
		want: `{"MyInt1":1,"MyInt2":3}`,
	}, {
		// If an anonymous struct pointer field is nil, we should ignore
		// the embedded fields behind it. Not properly doing so may
		// result in the wrong output or reflect panics.
		label: "EmbeddedFieldBehindNilPointer",
		makeInput: func() interface{} {
			type (
				S2 struct{ Field string }
				S  struct{ *S2 }
			)
			return S{}
		},
		want: `{}`,
	}}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			b, err := bson.Marshal(tt.makeInput())
			if err != nil {
				t.Fatalf("Marshal() = %v, want nil error", err)
			}
			if string(b) != tt.want {
				t.Fatalf("Marshal() = %q, want %q", b, tt.want)
			}
		})
	}
}

func TestInlineFieldsBSON(t *testing.T) {
	tests := []struct {
		label     string             // Test name
		makeInput func() interface{} // Function to create input value
		want      string             // Expected JSON output
	}{{
		// Both S1 and S2 have a field named X. From the perspective of S,
		// it is ambiguous which one X refers to.
		// This should not serialize either field.
		label: "AmbiguousField",
		makeInput: func() interface{} {
			type (
				S1 struct{ x, X int }
				S2 struct{ x, X int }
				S  struct {
					S1 S1 `json:",inline"`
					S2 S2 `json:",inline"`
				}
			)
			return S{S1{1, 2}, S2{3, 4}}
		},
		want: `{}`,
	}, {
		label: "DominantField",
		// Both S1 and S2 have a field named X, but since S has an X field as
		// well, it takes precedence over S1.X and S2.X.
		makeInput: func() interface{} {
			type (
				S1 struct{ x, X int }
				S2 struct{ x, X int }
				S  struct {
					S1   S1 `json:",inline"`
					S2   S2 `json:",inline"`
					x, X int
				}
			)
			return S{S1{1, 2}, S2{3, 4}, 5, 6}
		},
		want: `{"X":6}`,
	}, {
		// Unexported embedded field of non-struct type should not be serialized.
		label: "UnexportedEmbeddedInt",
		makeInput: func() interface{} {
			type (
				myInt int
				S     struct {
					myInt myInt `json:",inline"`
				}
			)
			return S{5}
		},
		want: `{}`,
	}, {
		// Exported embedded field of non-struct type should be serialized.
		label: "ExportedEmbeddedInt",
		makeInput: func() interface{} {
			type (
				MyInt int
				S     struct {
					MyInt MyInt `json:",inline"`
				}
			)
			return S{5}
		},
		want: `{"MyInt":5}`,
	}, {
		// Unexported embedded field of pointer to non-struct type
		// should not be serialized.
		label: "UnexportedEmbeddedIntPointer",
		makeInput: func() interface{} {
			type (
				myInt int
				S     struct {
					myInt *myInt `json:",inline"`
				}
			)
			s := S{new(myInt)}
			*s.myInt = 5
			return s
		},
		want: `{}`,
	}, {
		// Exported embedded field of pointer to non-struct type
		// should be serialized.
		label: "ExportedEmbeddedIntPointer",
		makeInput: func() interface{} {
			type (
				MyInt int
				S     struct {
					MyInt *MyInt `json:",inline"`
				}
			)
			s := S{new(MyInt)}
			*s.MyInt = 5
			return s
		},
		want: `{"MyInt":5}`,
	}, {
		// Exported fields of embedded structs should have their
		// exported fields be serialized regardless of whether the struct types
		// themselves are exported.
		label: "EmbeddedStruct",
		makeInput: func() interface{} {
			type (
				s1 struct{ x, X int }
				S2 struct{ y, Y int }
				S  struct {
					s1 s1 `json:",inline"`
					S2 S2 `json:",inline"`
				}
			)
			return S{s1{1, 2}, S2{3, 4}}
		},
		want: `{"X":2,"Y":4}`,
	}, {
		// Exported fields of pointers to embedded structs should have their
		// exported fields be serialized regardless of whether the struct types
		// themselves are exported.
		label: "EmbeddedStructPointer",
		makeInput: func() interface{} {
			type (
				s1 struct{ x, X int }
				S2 struct{ y, Y int }
				S  struct {
					s1 *s1 `json:",inline"`
					S2 *S2 `json:",inline"`
				}
			)
			return S{&s1{1, 2}, &S2{3, 4}}
		},
		want: `{"X":2,"Y":4}`,
	}, {
		// Exported fields on embedded unexported structs at multiple levels
		// of nesting should still be serialized.
		label: "NestedStructAndInts",
		makeInput: func() interface{} {
			type (
				MyInt1 int
				MyInt2 int
				myInt  int
				s2     struct {
					MyInt2 MyInt2 `json:",inline"`
					myInt  myInt  `json:",inline"`
				}
				s1 struct {
					MyInt1 MyInt1 `json:",inline"`
					myInt  myInt  `json:",inline"`
					s2     s2     `json:",inline"`
				}
				S struct {
					s1    s1    `json:",inline"`
					myInt myInt `json:",inline"`
				}
			)
			return S{s1{1, 2, s2{3, 4}}, 6}
		},
		want: `{"MyInt1":1,"MyInt2":3}`,
	}, {
		// If an anonymous struct pointer field is nil, we should ignore
		// the embedded fields behind it. Not properly doing so may
		// result in the wrong output or reflect panics.
		label: "EmbeddedFieldBehindNilPointer",
		makeInput: func() interface{} {
			type (
				S2 struct{ Field string }
				S  struct {
					S2 *S2 `json:",inline"`
				}
			)
			return S{}
		},
		want: `{}`,
	}}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			b, err := bson.Marshal(tt.makeInput())
			if err != nil {
				t.Fatalf("Marshal() = %v, want nil error", err)
			}
			if string(b) != tt.want {
				t.Fatalf("Marshal() = %q, want %q", b, tt.want)
			}
		})
	}
}

func TestInlineBSON(t *testing.T) {
	type args struct {
		v interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "tc 1",
			args: args{
				v: &Parent{
					Child: &Child{
						Value: "123",
					},
				},
			},
			want:    []byte(`{"value":"123"}`),
			wantErr: false,
		},
		{
			name: "tc 2",
			args: args{
				v: &Parent2{
					Child: &Child{
						Value: "123",
					},
				},
			},
			want:    []byte(`{"child":{"value":"123"}}`),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bson.Marshal(tt.args.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Marshal() = %v, want %v", string(got), string(tt.want))
			}
		})
	}
}

// See golang.org/issue/16042 and golang.org/issue/34235.
func TestNilMarshalBSON(t *testing.T) {
	testCases := []struct {
		v    interface{}
		want string
	}{
		{v: nil, want: `null`},
		{v: new(float64), want: `0`},
		{v: []interface{}(nil), want: `null`},
		{v: []string(nil), want: `null`},
		{v: map[string]string(nil), want: `null`},
		{v: []byte(nil), want: `null`},
		{v: struct{ M string }{"gopher"}, want: `{"M":"gopher"}`},
		{v: struct{ M Marshaler }{}, want: `{"M":null}`},
		{v: struct{ M Marshaler }{(*nilJSONMarshaler)(nil)}, want: `{"M":"0zenil0"}`},
		{v: struct{ M interface{} }{(*nilJSONMarshaler)(nil)}, want: `{"M":null}`},
		{v: struct{ M encoding.TextMarshaler }{}, want: `{"M":null}`},
		{v: struct{ M encoding.TextMarshaler }{(*nilTextMarshaler)(nil)}, want: `{"M":"0zenil0"}`},
		{v: struct{ M interface{} }{(*nilTextMarshaler)(nil)}, want: `{"M":null}`},
	}

	for _, tt := range testCases {
		out, err := bson.Marshal(tt.v)
		if err != nil || string(out) != tt.want {
			t.Errorf("Marshal(%#v) = %#q, %#v, want %#q, nil", tt.v, out, err, tt.want)
			continue
		}
	}
}

// Issue 5245.
func TestEmbeddedBugBSON(t *testing.T) {
	v := BugB{
		BugA{"A"},
		"B",
	}
	b, err := bson.Marshal(v)
	if err != nil {
		t.Fatal("Marshal:", err)
	}
	want := `{"S":"B"}`
	got := string(b)
	if got != want {
		t.Fatalf("Marshal: got %s want %s", got, want)
	}
	// Now check that the duplicate field, S, does not appear.
	x := BugX{
		A: 23,
	}
	b, err = bson.Marshal(x)
	if err != nil {
		t.Fatal("Marshal:", err)
	}
	want = `{"A":23}`
	got = string(b)
	if got != want {
		t.Fatalf("Marshal: got %s want %s", got, want)
	}
}

// Test that a field with a tag dominates untagged fields.
func TestTaggedFieldDominatesBSON(t *testing.T) {
	v := BugY{
		BugA{"BugA"},
		BugD{"BugD"},
	}
	b, err := bson.Marshal(v)
	if err != nil {
		t.Fatal("Marshal:", err)
	}
	want := `{"S":"BugD"}`
	got := string(b)
	if got != want {
		t.Fatalf("Marshal: got %s want %s", got, want)
	}
}

func TestDuplicatedFieldDisappearsBSON(t *testing.T) {
	v := BugZ{
		BugA{"BugA"},
		BugC{"BugC"},
		BugY{
			BugA{"nested BugA"},
			BugD{"nested BugD"},
		},
	}
	b, err := bson.Marshal(v)
	if err != nil {
		t.Fatal("Marshal:", err)
	}
	want := `{}`
	got := string(b)
	if got != want {
		t.Fatalf("Marshal: got %s want %s", got, want)
	}
}

func TestStringBytesBSON(t *testing.T) {
	t.Parallel()
	// Test that encodeState.stringBytes and encodeState.string use the same encoding.
	var r []rune
	for i := '\u0000'; i <= unicode.MaxRune; i++ {
		if testing.Short() && i > 1000 {
			i = unicode.MaxRune
		}
		r = append(r, i)
	}
	s := string(r) + "\xff\xff\xffhello" // some invalid UTF-8 too

	for _, escapeHTML := range []bool{true, false} {
		es := &encodeState{}
		es.string(s, escapeHTML)

		esBytes := &encodeState{}
		esBytes.stringBytes([]byte(s), escapeHTML)

		enc := es.Buffer.String()
		encBytes := esBytes.Buffer.String()
		if enc != encBytes {
			i := 0
			for i < len(enc) && i < len(encBytes) && enc[i] == encBytes[i] {
				i++
			}
			enc = enc[i:]
			encBytes = encBytes[i:]
			i = 0
			for i < len(enc) && i < len(encBytes) && enc[len(enc)-i-1] == encBytes[len(encBytes)-i-1] {
				i++
			}
			enc = enc[:len(enc)-i]
			encBytes = encBytes[:len(encBytes)-i]

			if len(enc) > 20 {
				enc = enc[:20] + "..."
			}
			if len(encBytes) > 20 {
				encBytes = encBytes[:20] + "..."
			}

			t.Errorf("with escapeHTML=%t, encodings differ at %#q vs %#q",
				escapeHTML, enc, encBytes)
		}
	}
}

func TestIssue10281BSON(t *testing.T) {
	type Foo struct {
		N Number
	}
	x := Foo{Number(`invalid`)}

	b, err := bson.Marshal(&x)
	if err == nil {
		t.Errorf("Marshal(&x) = %#q; want error", b)
	}
}

func TestHTMLEscapeBSON(t *testing.T) {
	var b, want bytes.Buffer
	m := `{"M":"<html>foo &` + "\xe2\x80\xa8 \xe2\x80\xa9" + `</html>"}`
	want.Write([]byte(`{"M":"\u003chtml\u003efoo \u0026\u2028 \u2029\u003c/html\u003e"}`))
	HTMLEscape(&b, []byte(m))
	if !bytes.Equal(b.Bytes(), want.Bytes()) {
		t.Errorf("HTMLEscape(&b, []byte(m)) = %s; want %s", b.Bytes(), want.Bytes())
	}
}

// golang.org/issue/8582
func TestEncodePointerStringBSON(t *testing.T) {
	type stringPointer struct {
		N *int64 `json:"n,string"`
	}
	var n int64 = 42
	b, err := bson.Marshal(stringPointer{N: &n})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(b), `{"n":"42"}`; got != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
	var back stringPointer
	err = Unmarshal(b, &back)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.N == nil {
		t.Fatalf("Unmarshaled nil N field")
	}
	if *back.N != 42 {
		t.Fatalf("*N = %d; want 42", *back.N)
	}
}

func TestEncodeStringBSON(t *testing.T) {
	for _, tt := range encodeStringTests {
		b, err := bson.Marshal(tt.in)
		if err != nil {
			t.Errorf("Marshal(%q): %v", tt.in, err)
			continue
		}
		out := string(b)
		if out != tt.out {
			t.Errorf("Marshal(%q) = %#q, want %#q", tt.in, out, tt.out)
		}
	}
}

// Issue 13783
func TestEncodeBytekindBSON(t *testing.T) {
	testdata := []struct {
		data interface{}
		want string
	}{
		{byte(7), "7"},
		{jsonbyte(7), `{"JB":7}`},
		{textbyte(4), `"TB:4"`},
		{jsonint(5), `{"JI":5}`},
		{textint(1), `"TI:1"`},
		{[]byte{0, 1}, `"AAE="`},
		{[]jsonbyte{0, 1}, `[{"JB":0},{"JB":1}]`},
		{[][]jsonbyte{{0, 1}, {3}}, `[[{"JB":0},{"JB":1}],[{"JB":3}]]`},
		{[]textbyte{2, 3}, `["TB:2","TB:3"]`},
		{[]jsonint{5, 4}, `[{"JI":5},{"JI":4}]`},
		{[]textint{9, 3}, `["TI:9","TI:3"]`},
		{[]int{9, 3}, `[9,3]`},
		{[]textfloat{12, 3}, `["TF:12.00","TF:3.00"]`},
	}
	for _, d := range testdata {
		js, err := bson.Marshal(d.data)
		if err != nil {
			t.Error(err)
			continue
		}
		got, want := string(js), d.want
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
}

func TestTextMarshalerMapKeysAreSortedBSON(t *testing.T) {
	b, err := bson.Marshal(map[unmarshalerText]int{
		{"x", "y"}: 1,
		{"y", "x"}: 2,
		{"a", "z"}: 3,
		{"z", "a"}: 4,
	})
	if err != nil {
		t.Fatalf("Failed to Marshal text.Marshaler: %v", err)
	}
	const want = `{"a:z":3,"x:y":1,"y:x":2,"z:a":4}`
	if string(b) != want {
		t.Errorf("Marshal map with text.Marshaler keys: got %#q, want %#q", b, want)
	}
}

// https://golang.org/issue/33675
func TestNilMarshalerTextMapKeyBSON(t *testing.T) {
	b, err := bson.Marshal(map[*unmarshalerText]int{
		(*unmarshalerText)(nil): 1,
		{"A", "B"}:              2,
	})
	if err != nil {
		t.Fatalf("Failed to Marshal *text.Marshaler: %v", err)
	}
	const want = `{"":1,"A:B":2}`
	if string(b) != want {
		t.Errorf("Marshal map with *text.Marshaler keys: got %#q, want %#q", b, want)
	}
}
func TestMarshalFloatBSON(t *testing.T) {
	t.Parallel()
	nfail := 0
	test := func(f float64, bits int) {
		vf := interface{}(f)
		if bits == 32 {
			f = float64(float32(f)) // round
			vf = float32(f)
		}
		bout, err := bson.Marshal(vf)
		if err != nil {
			t.Errorf("Marshal(%T(%g)): %v", vf, vf, err)
			nfail++
			return
		}
		out := string(bout)

		// result must convert back to the same float
		g, err := strconv.ParseFloat(out, bits)
		if err != nil {
			t.Errorf("Marshal(%T(%g)) = %q, cannot parse back: %v", vf, vf, out, err)
			nfail++
			return
		}
		if f != g || fmt.Sprint(f) != fmt.Sprint(g) { // fmt.Sprint handles ±0
			t.Errorf("Marshal(%T(%g)) = %q (is %g, not %g)", vf, vf, out, float32(g), vf)
			nfail++
			return
		}

		bad := badFloatREs
		if bits == 64 {
			bad = bad[:len(bad)-2]
		}
		for _, re := range bad {
			if re.MatchString(out) {
				t.Errorf("Marshal(%T(%g)) = %q, must not match /%s/", vf, vf, out, re)
				nfail++
				return
			}
		}
	}

	var (
		bigger  = math.Inf(+1)
		smaller = math.Inf(-1)
	)

	var digits = "1.2345678901234567890123"
	for i := len(digits); i >= 2; i-- {
		if testing.Short() && i < len(digits)-4 {
			break
		}
		for exp := -30; exp <= 30; exp++ {
			for _, sign := range "+-" {
				for bits := 32; bits <= 64; bits += 32 {
					s := fmt.Sprintf("%c%se%d", sign, digits[:i], exp)
					f, err := strconv.ParseFloat(s, bits)
					if err != nil {
						log.Fatal(err)
					}
					next := math.Nextafter
					if bits == 32 {
						next = func(g, h float64) float64 {
							return float64(math.Nextafter32(float32(g), float32(h)))
						}
					}
					test(f, bits)
					test(next(f, bigger), bits)
					test(next(f, smaller), bits)
					if nfail > 50 {
						t.Fatalf("stopping test early")
					}
				}
			}
		}
	}
	test(0, 64)
	test(math.Copysign(0, -1), 64)
	test(0, 32)
	test(math.Copysign(0, -1), 32)
}

func TestMarshalRawMessageValueBSON(t *testing.T) {
	type (
		T1 struct {
			M RawMessage `json:",omitempty"`
		}
		T2 struct {
			M *RawMessage `json:",omitempty"`
		}
	)

	var (
		rawNil   = RawMessage(nil)
		rawEmpty = RawMessage([]byte{})
		rawText  = RawMessage([]byte(`"foo"`))
	)

	tests := []struct {
		in   interface{}
		want string
		ok   bool
	}{
		// Test with nil RawMessage.
		{rawNil, "null", true},
		{&rawNil, "null", true},
		{[]interface{}{rawNil}, "[null]", true},
		{&[]interface{}{rawNil}, "[null]", true},
		{[]interface{}{&rawNil}, "[null]", true},
		{&[]interface{}{&rawNil}, "[null]", true},
		{struct{ M RawMessage }{rawNil}, `{"M":null}`, true},
		{&struct{ M RawMessage }{rawNil}, `{"M":null}`, true},
		{struct{ M *RawMessage }{&rawNil}, `{"M":null}`, true},
		{&struct{ M *RawMessage }{&rawNil}, `{"M":null}`, true},
		{map[string]interface{}{"M": rawNil}, `{"M":null}`, true},
		{&map[string]interface{}{"M": rawNil}, `{"M":null}`, true},
		{map[string]interface{}{"M": &rawNil}, `{"M":null}`, true},
		{&map[string]interface{}{"M": &rawNil}, `{"M":null}`, true},
		{T1{rawNil}, "{}", true},
		{T2{&rawNil}, `{"M":null}`, true},
		{&T1{rawNil}, "{}", true},
		{&T2{&rawNil}, `{"M":null}`, true},

		// Test with empty, but non-nil, RawMessage.
		{rawEmpty, "", false},
		{&rawEmpty, "", false},
		{[]interface{}{rawEmpty}, "", false},
		{&[]interface{}{rawEmpty}, "", false},
		{[]interface{}{&rawEmpty}, "", false},
		{&[]interface{}{&rawEmpty}, "", false},
		{struct{ X RawMessage }{rawEmpty}, "", false},
		{&struct{ X RawMessage }{rawEmpty}, "", false},
		{struct{ X *RawMessage }{&rawEmpty}, "", false},
		{&struct{ X *RawMessage }{&rawEmpty}, "", false},
		{map[string]interface{}{"nil": rawEmpty}, "", false},
		{&map[string]interface{}{"nil": rawEmpty}, "", false},
		{map[string]interface{}{"nil": &rawEmpty}, "", false},
		{&map[string]interface{}{"nil": &rawEmpty}, "", false},
		{T1{rawEmpty}, "{}", true},
		{T2{&rawEmpty}, "", false},
		{&T1{rawEmpty}, "{}", true},
		{&T2{&rawEmpty}, "", false},

		// Test with RawMessage with some text.
		//
		// The tests below marked with Issue6458 used to generate "ImZvbyI=" instead "foo".
		// This behavior was intentionally changed in Go 1.8.
		// See https://golang.org/issues/14493#issuecomment-255857318
		{rawText, `"foo"`, true}, // Issue6458
		{&rawText, `"foo"`, true},
		{[]interface{}{rawText}, `["foo"]`, true},  // Issue6458
		{&[]interface{}{rawText}, `["foo"]`, true}, // Issue6458
		{[]interface{}{&rawText}, `["foo"]`, true},
		{&[]interface{}{&rawText}, `["foo"]`, true},
		{struct{ M RawMessage }{rawText}, `{"M":"foo"}`, true}, // Issue6458
		{&struct{ M RawMessage }{rawText}, `{"M":"foo"}`, true},
		{struct{ M *RawMessage }{&rawText}, `{"M":"foo"}`, true},
		{&struct{ M *RawMessage }{&rawText}, `{"M":"foo"}`, true},
		{map[string]interface{}{"M": rawText}, `{"M":"foo"}`, true},  // Issue6458
		{&map[string]interface{}{"M": rawText}, `{"M":"foo"}`, true}, // Issue6458
		{map[string]interface{}{"M": &rawText}, `{"M":"foo"}`, true},
		{&map[string]interface{}{"M": &rawText}, `{"M":"foo"}`, true},
		{T1{rawText}, `{"M":"foo"}`, true}, // Issue6458
		{T2{&rawText}, `{"M":"foo"}`, true},
		{&T1{rawText}, `{"M":"foo"}`, true},
		{&T2{&rawText}, `{"M":"foo"}`, true},
	}

	for i, tt := range tests {
		b, err := bson.Marshal(tt.in)
		if ok := (err == nil); ok != tt.ok {
			if err != nil {
				t.Errorf("test %d, unexpected failure: %v", i, err)
			} else {
				t.Errorf("test %d, unexpected success", i)
			}
		}
		if got := string(b); got != tt.want {
			t.Errorf("test %d, bson.Marshal(%#v) = %q, want %q", i, tt.in, got, tt.want)
		}
	}
}

func TestMarshalPanicBSON(t *testing.T) {
	defer func() {
		if got := recover(); !reflect.DeepEqual(got, 0xdead) {
			t.Errorf("panic() = (%T)(%v), want 0xdead", got, got)
		}
	}()
	Marshal(&marshalPanic{})
	t.Error("Marshal should have panicked")
}

func TestMarshalUncommonFieldNamesBSON(t *testing.T) {
	v := struct {
		A0, À, Aβ int
	}{}
	b, err := bson.Marshal(v)
	if err != nil {
		t.Fatal("Marshal:", err)
	}
	want := `{"A0":0,"À":0,"Aβ":0}`
	got := string(b)
	if got != want {
		t.Fatalf("Marshal: got %s want %s", got, want)
	}
}

func TestMarshalerErrorBSON(t *testing.T) {
	s := "test variable"
	st := reflect.TypeOf(s)
	errText := "json: test error"

	tests := []struct {
		err  *MarshalerError
		want string
	}{
		{
			&MarshalerError{st, fmt.Errorf(errText), ""},
			"json: error calling MarshalJSON for type " + st.String() + ": " + errText,
		},
		{
			&MarshalerError{st, fmt.Errorf(errText), "TestMarshalerError"},
			"json: error calling TestMarshalerError for type " + st.String() + ": " + errText,
		},
	}

	for i, tt := range tests {
		got := tt.err.Error()
		if got != tt.want {
			t.Errorf("MarshalerError test %d, got: %s, want: %s", i, got, tt.want)
		}
	}
}
