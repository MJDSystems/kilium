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
package kilium

type Comparable interface {
	Less(right Comparable) bool
}

type ComparableArray interface {
	Make() ComparableArray
	Append(Comparable)
	Get(index int) Comparable
	RemoveAt(index int)
	Len() int
}

// Returns the merged result of x/y.  It expects a sorted x/y, in descending order.  It returns a descending
// order sorted array with x/y de-duped.
func InsertSliceSort(x, y ComparableArray) ComparableArray {
	ret := x.Make()

	xI, yI := 0, 0
	for xI < x.Len() && yI < y.Len() {
		nextX := x.Get(xI)
		nextY := y.Get(yI)
		if nextY.Less(nextX) { //Remember, need X bigger then Y to be next, since it is all in descending order.
			ret.Append(nextX)
			xI++
		} else if nextX.Less(nextY) { // This means Y is bigger, and is next to append.
			ret.Append(nextY)
			yI++
		} else {
			ret.Append(nextX) // Randomly choosen to be from the first list
			xI++
			yI++
		}
	}

	for ; xI < x.Len(); xI++ {
		ret.Append(x.Get(xI))
	}

	for ; yI < y.Len(); yI++ {
		ret.Append(y.Get(yI))
	}

	return ret
}

func RemoveSliceElements(elem, toRemove ComparableArray) {
	for eI, rI := 0, 0; eI < elem.Len() && rI < toRemove.Len(); {
		nextE := elem.Get(eI)
		nextR := toRemove.Get(rI)
		if nextR.Less(nextE) {
			eI++ //This element is not the one.  Next!
		} else if nextE.Less(nextR) {
			rI++ //This is not in the list.  Next!
		} else {
			//Dup!
			elem.RemoveAt(eI)
			rI++
		}
	}
}
