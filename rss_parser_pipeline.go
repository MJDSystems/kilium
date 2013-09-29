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

	riak "github.com/tpjg/goriakpbc"
)

type RssParserPipeline struct {
	InputCh  chan<- url.URL
	OutputCh <-chan FeedError

	fetcherCh    chan url.URL
	parserCh     chan RawFeed
	updateDbDch  chan FeedParserOut
	completionCh chan UpdatedModel
}

func rssParserPipelineFinishItem(completionCh chan UpdatedModel, outputCh chan FeedError) {
	for {
		if output, ok := <-completionCh; ok {
			outputCh <- FeedError{Url: output.Url, Err: nil}
		} else {
			break
		}
	}
}

func NewRssParserPipeline(con *riak.Client, idGenerator <-chan uint64) (pipeline RssParserPipeline) {
	InputCh := make(chan url.URL)
	OutputCh := make(chan FeedError)
	pipeline = RssParserPipeline{
		InputCh:  InputCh,
		OutputCh: OutputCh,

		parserCh:     make(chan RawFeed),
		updateDbDch:  make(chan FeedParserOut),
		completionCh: make(chan UpdatedModel),
	}

	// Launch the various pipeline pieces.
	go FeedFetcher(InputCh, pipeline.parserCh, OutputCh)
	go FeedParser(pipeline.parserCh, pipeline.updateDbDch, OutputCh)
	go UpdateFeed(con, idGenerator, pipeline.updateDbDch, pipeline.completionCh, OutputCh)

	// Launch the handling go routines.
	go rssParserPipelineFinishItem(pipeline.completionCh, OutputCh)

	return
}
