//go:build geas

package geas

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// buildContract compiles a contract into a scratch directory and returns the module path, skipping without the compiler
func buildContract(t *testing.T, src string) string {
	t.Helper()
	if _, err := exec.LookPath("geas"); err != nil {
		t.Skip("geas not on PATH")
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cmd := exec.Command("geas", "build", abs)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("geas build: %v\n%s", err, out)
	}
	stem := filepath.Base(src)
	stem = stem[:len(stem)-len(filepath.Ext(stem))]
	return filepath.Join(dir, "target", "geas-out", "lib"+stem+".geas.so")
}

// echoRuntime brings up a runtime with the echo module loaded and total bound in Go
func echoRuntime(t *testing.T) *Runtime {
	t.Helper()
	so := buildContract(t, "testdata/echo.geas")
	rt, err := Init()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Shutdown)
	if err := rt.Load(so); err != nil {
		t.Fatal(err)
	}
	// total sums a list of ints in Go
	err = rt.Bind("Echo.total", func(_ *Instance, args []Value) (Value, error) {
		var sum int64
		for _, x := range args[0].(List).Items {
			sum += x.(int64)
		}
		return Result{Ok: true, Value: sum}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestSignReadsVowsAndDefaults(t *testing.T) {
	rt := echoRuntime(t)
	inst, err := rt.Sign("Echo", map[string]Value{"tagline": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := inst.Vow("tagline"); v != "hi" {
		t.Fatalf("tagline = %v", v)
	}
	if v, _ := inst.Vow("limit"); v != int64(3) {
		t.Fatalf("limit = %v", v)
	}
	if inst.State() != Signed || inst.State().String() != "Signed" {
		t.Fatalf("state = %v", inst.State())
	}
}

func TestSignRejectsBadVow(t *testing.T) {
	rt := echoRuntime(t)
	if _, err := rt.Sign("Echo", map[string]Value{"nope": "x"}); err == nil {
		t.Fatal("expected error for unknown vow")
	}
	if _, err := rt.Sign("Echo", map[string]Value{"limit": "not an int"}); err == nil {
		t.Fatal("expected error for wrong vow type")
	}
	if _, err := rt.Sign("Ghost", nil); err == nil {
		t.Fatal("expected error for unknown contract")
	}
}

func TestStringsAndVowInBody(t *testing.T) {
	rt := echoRuntime(t)
	inst, _ := rt.Sign("Echo", map[string]Value{"tagline": "hello"})
	v, err := inst.Fulfill("greet", "world")
	if err != nil {
		t.Fatal(err)
	}
	r, ok := v.(Result)
	if !ok || !r.Ok || r.Value != "hello world" {
		t.Fatalf("greet = %#v", v)
	}
}

func TestRecordsAndLists(t *testing.T) {
	rt := echoRuntime(t)
	inst, _ := rt.Sign("Echo", nil)
	pair := Record{Fields: []Value{"apples", int64(4)}}
	v, err := inst.Fulfill("twice", pair)
	if err != nil {
		t.Fatal(err)
	}
	r := v.(Result)
	list, ok := r.Value.(List)
	if !r.Ok || !ok || len(list.Items) != 2 || list.Elem != TagRecord {
		t.Fatalf("twice = %#v", v)
	}
	got := list.Items[1].(Record)
	if got.Fields[0] != "apples" || got.Fields[1] != int64(4) {
		t.Fatalf("record = %#v", got)
	}
}

func TestSumErrorAndPartialErrors(t *testing.T) {
	rt := echoRuntime(t)
	inst, _ := rt.Sign("Echo", nil)
	v, err := inst.Fulfill("fail", "broken disk")
	if err != nil {
		t.Fatal(err)
	}
	r := v.(Result)
	s, ok := r.Value.(Sum)
	if r.Ok || !ok || s.Tag != 1 || len(s.Fields) != 1 || s.Fields[0] != "broken disk" {
		t.Fatalf("fail = %#v", v)
	}
	errs := inst.Errors()
	if len(errs) != 1 || errs[0].Pledge != "fail" {
		t.Fatalf("errors = %#v", errs)
	}
	if got := inst.Items(ItemBroken); len(got) != 1 || got[0] != "fail" {
		t.Fatalf("broken items = %v", got)
	}
}

func TestOptions(t *testing.T) {
	rt := echoRuntime(t)
	inst, _ := rt.Sign("Echo", map[string]Value{"limit": int64(9)})
	v, _ := inst.Fulfill("maybe", true)
	if o := v.(Option); !o.Some || o.Value != int64(9) {
		t.Fatalf("maybe true = %#v", v)
	}
	v, _ = inst.Fulfill("maybe", false)
	if o := v.(Option); o.Some {
		t.Fatalf("maybe false = %#v", v)
	}
}

func TestBoundGoPledge(t *testing.T) {
	rt := echoRuntime(t)
	inst, _ := rt.Sign("Echo", nil)
	v, err := inst.Fulfill("total", List{Elem: TagInt, Items: []Value{int64(1), int64(2), int64(3)}})
	if err != nil {
		t.Fatal(err)
	}
	if r := v.(Result); !r.Ok || r.Value != int64(6) {
		t.Fatalf("total = %#v", v)
	}
	if got := inst.Items(ItemFulfilled); len(got) != 1 || got[0] != "total" {
		t.Fatalf("fulfilled items = %v", got)
	}
}

func TestStatusErrors(t *testing.T) {
	rt := echoRuntime(t)
	inst, _ := rt.Sign("Echo", nil)
	if _, err := inst.Fulfill("ghost"); err == nil {
		t.Fatal("expected error for unknown pledge")
	}
	if _, err := inst.Fulfill("greet", int64(1)); err == nil {
		t.Fatal("expected error for wrong argument type")
	}
	if err := inst.Break(); err != nil {
		t.Fatal(err)
	}
	if inst.State() != Broken {
		t.Fatalf("state after break = %v", inst.State())
	}
	if _, err := inst.Fulfill("greet", "x"); err == nil {
		t.Fatal("expected error after break")
	}
}
