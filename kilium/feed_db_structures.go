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
	"bytes"
	"encoding/binary"

	"encoding/base64"

	"net/url"
	"strconv"
	"time"

	riak "github.com/tpjg/goriakpbc"
)

type Feed struct {
	Url url.URL `riak:"url"`

	Title     string    `riak:"title"`
	LastCheck time.Time `riak:"last_check"`

	ItemKeys         ItemKeyList `riak:"item_keys"`
	InsertedItemKeys ItemKeyList `riak:"inserted_items"`
	DeletedItemKeys  ItemKeyList `riak:"deleted_items"`

	NextCheck time.Time `riak:"next_check"`

	riak.Model `riak:"feeds"`
}

type FeedItem struct {
	Title   string `riak:"title"`
	Author  string `riak:"author"`
	Content string `riak:"content"`

	Url url.URL `riak:"url"`

	PubDate time.Time `riak:"publication_date"`

	riak.Model `riak:"items"`
}

const NextCheckIndexName = "next_check_int"

type ItemKey []byte

func NewItemKey(id uint64, rawId []byte) ItemKey {
	buf := &bytes.Buffer{}

	binary.Write(buf, binary.BigEndian, id)
	buf.WriteString("-")
	buf.Write(rawId)

	return buf.Bytes()
}

func (l ItemKey) Less(r Comparable) bool {
	return bytes.Compare(l, r.(ItemKey)) == -1
}

func (l ItemKey) Equal(r ItemKey) bool {
	return bytes.Compare(l, r) == 0
}

func (l *ItemKey) UnmarshalJSON(input []byte) error {
	input = input[1 : len(input)-1]

	*l = make(ItemKey, base64.URLEncoding.DecodedLen(len(input)))
	n, err := base64.URLEncoding.Decode(*l, input)

	if err != nil {
		return err
	} else {
		*l = (*l)[0:n]
		return nil
	}
}

func (l ItemKey) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString("\"")

	dst := make([]byte, base64.StdEncoding.EncodedLen(len(l)))
	base64.URLEncoding.Encode(dst, l)
	buffer.Write(dst)

	buffer.WriteByte('"')
	return buffer.Bytes(), nil
}

func (key ItemKey) IsRawItemId(id []byte) bool {
	return bytes.Compare(key[9:], id) == 0
}

func (key ItemKey) GetRiakKey() string {
	return base64.URLEncoding.EncodeToString(key)
}

type ItemKeyList []ItemKey

func (list *ItemKeyList) Append(key Comparable) {
	*list = append(*list, key.(ItemKey))
}

func (list ItemKeyList) Get(index int) Comparable {
	return list[index]
}

func (list *ItemKeyList) RemoveAt(index int) {
	*list = append((*list)[:index], (*list)[index+1:]...)
}

func (list ItemKeyList) Len() int {
	return len(list)
}

func (list ItemKeyList) Less(l, r int) bool {
	return list[l].Less(list[r])
}

func (list ItemKeyList) Swap(l, r int) {
	list[l], list[r] = list[r], list[l]
}

func (ItemKeyList) Make() ComparableArray {
	return &ItemKeyList{}
}

func (list ItemKeyList) FindRawItemId(id []byte) int {
	for i, key := range list {
		if key.IsRawItemId(id) {
			return i
		}
	}
	return -1
}

func (f *Feed) UrlKey() string {
	return base64.URLEncoding.EncodeToString(makeHash(f.Url.String()))
}

func (f *Feed) Resolve(siblingsCount int) error {
	// First get the siblings!
	siblingsI, err := f.Siblings(&Feed{})
	if err != nil {
		return err
	}
	siblings := siblingsI.([]Feed)

	// Set the Url, as it is constant
	f.Url = siblings[0].Url

	for i := 0; i < siblingsCount; i++ {
		// Resolve regular feed details.  Basically, take the latest version!
		// If this is the first object, it will have a zero time of year 1st.  If a feed is claiming
		// be older then that, well it just won't work.
		if i == 0 || siblings[i].LastCheck.After(f.LastCheck) {
			f.Title = siblings[i].Title
			f.LastCheck = siblings[i].LastCheck
			f.NextCheck = siblings[i].NextCheck
		}

		// for the item lists, merge and de-dup using insert slice sort!
		f.ItemKeys = *InsertSliceSort(&f.ItemKeys, &siblings[i].ItemKeys).(*ItemKeyList)
		f.InsertedItemKeys = *InsertSliceSort(&f.InsertedItemKeys, &siblings[i].InsertedItemKeys).(*ItemKeyList)
		f.DeletedItemKeys = *InsertSliceSort(&f.DeletedItemKeys, &siblings[i].DeletedItemKeys).(*ItemKeyList)
	}
	RemoveSliceElements(&f.InsertedItemKeys, &f.ItemKeys)
	RemoveSliceElements(&f.InsertedItemKeys, &f.DeletedItemKeys)
	RemoveSliceElements(&f.ItemKeys, &f.DeletedItemKeys)

	// Since this index just matches a data field, just use that.
	f.Indexes()[NextCheckIndexName] = strconv.FormatInt(f.NextCheck.Unix(), 10)

	return nil
}

func (f *FeedItem) Resolve(siblingsCount int) error {
	// First get the siblings!
	siblingsI, err := f.Siblings(&FeedItem{})
	if err != nil {
		return err
	}
	siblings := siblingsI.([]FeedItem)

	for i := 0; i < siblingsCount; i++ {
		// Feed items are simple.  What ever claims is the latest update wins.  This should come from
		// the feed when possible.  Otherwise it is generated by the system, but should still be ok.
		if i == 0 || siblings[i].PubDate.After(f.PubDate) {
			f.Title = siblings[i].Title
			f.Author = siblings[i].Author
			f.Content = siblings[i].Content
			f.Url = siblings[i].Url
			f.PubDate = siblings[i].PubDate
		}
	}

	///@todo: I need to merge indexes here.  Currently I can't though due to library restrictions.

	return nil
}
