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
	"time"

	riak "github.com/tpjg/goriakpbc"
)

type Feed struct {
	Url url.URL `riak:"url"`

	Title     string    `riak:"title"`
	LastCheck time.Time `riak:"last_check"`

	ItemKeys        []byte `riak:"item_keys"`
	DeletedItemKeys []byte `riak:"deleted_items"`

	NextCheck time.Time `riak:"next_check"`

	riak.Model
}

type FeedItem struct {
	riak.Model
}
