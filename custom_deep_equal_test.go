// Copyright 2009 The Go Authors. All rights reserved.
// Copyright (C) 2013 Matthew Dawson <matthew@mjdsystems.ca>
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Deep equality test via reflection
// Modified to include a skip fields system for structs.

package main

import "reflect"

// During deepValueEqual, must keep track of checks that are
// in progress.  The comparison algorithm assumes that all
// checks in progress are true when it reencounters them.
// Visited are stored in a map indexed by 17 * a1 + a2;
type visit struct {
	a1   uintptr
	a2   uintptr
	typ  reflect.Type
	next *visit
}

// Tests for deep equality using reflected types. The map argument tracks
// comparisons that have already been seen, which allows short circuiting on
// recursive types.
func deepValueEqual(v1, v2 reflect.Value, skipFields []string, visited map[uintptr]*visit, depth int) (b bool) {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}

	// if depth > 10 { panic("deepValueEqual") }	// for debugging

	if v1.CanAddr() && v2.CanAddr() {
		addr1 := v1.UnsafeAddr()
		addr2 := v2.UnsafeAddr()
		if addr1 > addr2 {
			// Canonicalize order to reduce number of entries in visited.
			addr1, addr2 = addr2, addr1
		}

		// Short circuit if references are identical ...
		if addr1 == addr2 {
			return true
		}

		// ... or already seen
		h := 17*addr1 + addr2
		seen := visited[h]
		typ := v1.Type()
		for p := seen; p != nil; p = p.next {
			if p.a1 == addr1 && p.a2 == addr2 && p.typ == typ {
				return true
			}
		}

		// Remember for later.
		visited[h] = &visit{addr1, addr2, typ, seen}
	}

	// Really new magic: If the type has a method called equal that accepts the *exact* type and not
	// an interface{} (for now), and returns bool, then call that to check equality instead of doing
	// below.  Used to work around time's location field, for instance, since it doens't matter.
	if method, ok := v1.Type().MethodByName("Equal"); ok {
		// Found a potential.  Now verify other constraints
		methodType := method.Type
		if methodType.NumIn() == 2 && methodType.In(0) == v1.Type() && methodType.In(1) == v2.Type() &&
			methodType.NumOut() == 1 && methodType.Out(0).Kind() == reflect.Bool {
			// And it matches.  Call it and return the result.
			return method.Func.Call([]reflect.Value{v1, v2})[0].Bool()
		}
	}

	switch v1.Kind() {
	case reflect.Array:
		if v1.Len() != v2.Len() {
			return false
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), skipFields, visited, depth+1) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), skipFields, visited, depth+1) {
				return false
			}
		}
		return true
	case reflect.Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return deepValueEqual(v1.Elem(), v2.Elem(), skipFields, visited, depth+1)
	case reflect.Ptr:
		return deepValueEqual(v1.Elem(), v2.Elem(), skipFields, visited, depth+1)
	case reflect.Struct:
		for i, n := 0, v1.NumField(); i < n; i++ {
			// Skip fields as requested!
			skip := false
			for _, skipField := range skipFields {
				if v1.Type().Field(i).Name == skipField {
					skip = true
					break
				}
			}
			if skip {
				continue
			}

			if !deepValueEqual(v1.Field(i), v2.Field(i), skipFields, visited, depth+1) {
				return false
			}
		}
		return true
	case reflect.Map:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		for _, k := range v1.MapKeys() {
			if !deepValueEqual(v1.MapIndex(k), v2.MapIndex(k), skipFields, visited, depth+1) {
				return false
			}
		}
		return true
	case reflect.Func:
		if v1.IsNil() && v2.IsNil() {
			return true
		}
		// Can't do better than this:
		return false

	// Normal equality is fine here.  However, if the field is unexported, I can't cheat like the
	// reflect package can.  Thus do direct comparisions below for more concrete types as I find the
	// case for them.  Fall back to Interface() as a last resort.
	case reflect.Int64, reflect.Int32:
		return v1.Int() == v2.Int()
	case reflect.Uint32:
		return v1.Uint() == v2.Uint()
	case reflect.String:
		return v1.String() == v2.String()
	default:
		// Normal equality suffices
		return v1.Interface() == v2.Interface()
	}
}

// DeepEqual tests for deep equality. It uses normal == equality where
// possible but will scan elements of arrays, slices, maps, and fields of
// structs. In maps, keys are compared with == but elements use deep
// equality. DeepEqual correctly handles recursive types. Functions are equal
// only if they are both nil.
// An empty slice is not equal to a nil slice.
func customDeepEqual(a1, a2 interface{}, skipFields []string) bool {
	if a1 == nil || a2 == nil {
		return a1 == a2
	}
	v1 := reflect.ValueOf(a1)
	v2 := reflect.ValueOf(a2)
	if v1.Type() != v2.Type() {
		return false
	}
	return deepValueEqual(v1, v2, skipFields, make(map[uintptr]*visit), 0)
}
