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
	"net/url"

	"testing"

	"time"

	_ "github.com/tpjg/goriakpbc"
)

var testFeedUrl, _ = url.Parse("http://example.com/feed.rss")

func compareFeeds(f1, f2 Feed) bool {
	return customDeepEqual(f1, f2, []string{"Model"})
}

func TestFeedAtributesResolving(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	EntryA := Feed{
		Url: *testFeedUrl,

		Title:     "First Title",
		LastCheck: time.Date(2013, 7, 1, 0, 0, 0, 0, time.UTC),

		NextCheck: time.Date(2013, 7, 1, 1, 0, 0, 0, time.UTC),
		//No items, because I don't care.
	}
	EntryB := Feed{
		Url: *testFeedUrl,

		Title:     "Second Title",
		LastCheck: time.Date(2013, 7, 1, 12, 0, 0, 0, time.UTC), //This is of course a second check date.

		NextCheck: time.Date(2013, 7, 1, 13, 0, 0, 0, time.UTC),
		//No items, because I don't care.
	}

	if err := con.NewModel("ConflictFeed", &EntryA); err != nil {
		t.Fatalf("Failed to create EntryA's model (%s)", err)
	}
	if err := EntryA.Save(); err != nil {
		t.Fatalf("Failed to save EntryA (%s)", err)
	}

	if err := con.NewModel("ConflictFeed", &EntryB); err != nil {
		t.Fatalf("Failed to create EntryB's model (%s)", err)
	}
	if err := EntryB.Save(); err != nil {
		t.Fatalf("Failed to save EntryB (%s)", err)
	}

	// Cause conflict
	load := Feed{}
	if err := con.LoadModel("ConflictFeed", &load); err != nil {
		t.Fatalf("Failed to load conflict model  (%s)", err)
	} else if compareFeeds(EntryB, load) == false {
		t.Errorf("Resolved model does not match latest update (old, new) (%v, %v)", EntryB, load)
	}
}