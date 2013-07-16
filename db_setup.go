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
	"github.com/tpjg/goriakpbc"
)

func setupBucket(cli *riak.Client, bucketName string) error {
	bucket, err := cli.NewBucket(bucketName)
	if err != nil {
		return err
	}
	err = bucket.SetAllowMult(true)
	if err != nil {
		return err
	}
	return nil
}

func GetDatabaseConnection(addr string) (*riak.Client, error) {
	cli := riak.NewClientPool(addr, 100)
	err := cli.Connect()
	if err != nil {
		return nil, err
	}

	//Setup the buckets here.  For now we have feeds and items.  Make the multi set, but leave N at 3.
	err = setupBucket(cli, "feeds")
	if err != nil {
		return nil, err
	}

	err = setupBucket(cli, "items")
	if err != nil {
		return nil, err
	}

	return cli, nil
}
