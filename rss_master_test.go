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
	"time"

	"testing"
)

func TestRssMasterHandleAddRequest(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	Url := getUniqueExampleComUrl(t)

	loadFeed := &Feed{Url: *Url}

	if err := RssMasterHandleAddRequest(con, *Url); err != nil {
		t.Fatalf("Failed to create fresh new feed! (%s)", err)
	} else if err = con.LoadModel(loadFeed.UrlKey(), loadFeed); err != nil {
		t.Errorf("Failed to load just created model! (%s)", err)
	} else if !loadFeed.NextCheck.IsZero() {
		t.Errorf("Next check time on empty feed isn't zero! (Is: %s)", loadFeed.NextCheck)
	}

	loadFeed.NextCheck = loadFeed.NextCheck.Add(time.Hour * 24 * 365 * 200) //Bring us to something more recent.
	if err := loadFeed.Save(); err != nil {
		t.Fatalf("Failed to change the feed!")
	}

	loadFeed = &Feed{Url: *Url} // Reset the loaded feed

	if err := RssMasterHandleAddRequest(con, *Url); err != nil {
		t.Errorf("Failed to re-request new feed! (%s)", err)
	} else if err = con.LoadModel(loadFeed.UrlKey(), loadFeed); err != nil {
		t.Errorf("Failed to load new feed! (%s)", err)
	} else if loadFeed.NextCheck.IsZero() {
		t.Errorf("Next check time on empty feed is zero! (Is: %s)", loadFeed.NextCheck)
	}
}
