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

import (
	"time"

	"sync"
	"testing"

	riak "github.com/tpjg/goriakpbc"
)

var staticTestCon *riak.Client

// This is a static string that changes on every invocation of the test suite.  It is meant to ensure
// the keys are unique per run.
var key_uniquer string

func getTestConnection(t *testing.T) *riak.Client {
	if staticTestCon == nil {
		var err error
		staticTestCon, err = GetDatabaseConnection("localhost:8087")
		if err != nil {
			t.Fatalf("Failed to get db connection (%s)", err)
		}
		key_uniquer = time.Now().String()
	}
	return staticTestCon //This will only return if the fatal didn't happen.
}

func killBucket(con *riak.Client, bucketName string) error {
	bucket, err := con.NewBucket(bucketName)
	if err != nil {
		return err
	}

	keys, err := bucket.ListKeys()
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(len(keys))
	for _, key := range keys {
		go func(key string) {
			defer wg.Done()
			obj, err := bucket.Get(string(key))
			if obj == nil {
				panic(err)
			}
			err = obj.Destroy()
			if err != nil {
				panic(err)
			}
		}(string(key))
	}
	wg.Wait()

	return nil
}

func killTestDb(con *riak.Client, t *testing.T) {
	if err := killBucket(con, "feeds"); err != nil {
		t.Fatalf("Failed to kill bucket feeds (%s)", err)
	}
	if err := killBucket(con, "items"); err != nil {
		t.Fatalf("Failed to kill items feeds (%s)", err)
	}
}

func TestBucketsAfterConnect(t *testing.T) {
	con := getTestConnection(t)
	defer killTestDb(con, t)

	if feedsBucket, err := con.NewBucket("feeds"); err != nil {
		t.Errorf("Failed to get feeds bucket(%s)", err)
	} else if feedsBucket.AllowMult() != true {
		t.Error("Feeds bucket is not marked for multiple items!")
	}

	if itemsBucket, err := con.NewBucket("items"); err != nil {
		t.Errorf("Failed to get items bucket(%s)", err)
	} else if itemsBucket.AllowMult() != true {
		t.Error("Items bucket is not marked for multiple items!")
	}
}
