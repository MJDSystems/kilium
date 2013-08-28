package kiliumtmp

import (
	"log"
	"net/url"
)

var FeedList []url.URL

func setupFeedList(feeds []string) {
	for _, feed := range feeds {
		if Url, err := url.Parse(feed); err != nil {
			log.Fatalf("")
		} else {
			FeedList = append(FeedList, *Url)
		}
	}
}
