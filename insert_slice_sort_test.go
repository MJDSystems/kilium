/*
 * Copyright (C) 2013 Matthew Dawson <matthew@mjdsystems.ca>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package main

import (
	"math/rand"

	"reflect"

	"sort"

	"testing"
	"testing/quick"
)

type testInt int

func (l testInt) Less(r Comparable) bool {
	return l < r.(testInt)
}

func (testInt) Generate(rand *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(testInt(rand.Int()))
}

type testIntSlice []testInt

func (testIntSlice) Make() ComparableArray {
	ret := make(testIntSlice, 0)
	return &ret
}

func (s *testIntSlice) Append(elm Comparable) {
	*s = append(*s, elm.(testInt))
}
func (s testIntSlice) Get(index int) Comparable {
	return s[index]
}
func (s testIntSlice) Len() int {
	return len(s)
}

func (s testIntSlice) Less(i, j int) bool {
	return s[i] > s[j]
}

func (s testIntSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func PreformTest(t *testing.T, x, y testIntSlice) bool {
	//Insret Slice Sort expects sorted values.  So sort x/y
	sort.Sort(x)
	sort.Sort(y)

	ret := *InsertSliceSort(&x, &y).(*testIntSlice)

	var xI, yI int
	for i := 0; i < len(ret); i++ {
		if i+1 < len(ret) && ret[i] < ret[i+1] { // Needs to be descending order, thus next element is smaller.
			t.Logf("Falied to return sorted list at %v,%v %v", i, i+1, ret)
			return false
		}
		if xI < len(x) && x[xI] == ret[i] {
			xI++
		}
		if yI < len(y) && y[yI] == ret[i] {
			yI++
		}
	}
	return xI == len(x) && yI == len(y)
}

func TestInsertSliceSortUsingRandomInt(t *testing.T) {
	if err := quick.Check(func(x, y testIntSlice) bool { return PreformTest(t, x, y) }, nil); err != nil {
		t.Error(err)
	}
}

func TestInsertSliceSortUsingSameLists(t *testing.T) {
	if PreformTest(t, testIntSlice{4, 6}, testIntSlice{4, 6}) != true {
		t.Error("Falied to properly merge slices with the same content!")
	}
}
