package parser

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/tidwall/gjson"
)

type xianyu struct{}

func (x xianyu) parseVideoID(videoId string) (*VideoParseInfo, error) {
	return nil, errors.New("not implemented")
}

func (x xianyu) parseShareUrl(shareUrl string) (*VideoParseInfo, error) {
	client := resty.New()
	res, err := client.R().
		SetHeader(HttpHeaderUserAgent, DefaultUserAgent).
		Get(shareUrl)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`var url = '(.*?)';`)
	matches := re.FindStringSubmatch(string(res.Body()))
	if len(matches) < 2 {
		return nil, errors.New("could not find final URL in JavaScript variable")
	}

	finalUrl := matches[1]

	if !strings.Contains(finalUrl, "goofish.com") {
		return nil, fmt.Errorf("not a goofish.com url: %s", finalUrl)
	}

	return x.parseItemUrl(finalUrl)
}

func (x xianyu) parseItemUrl(itemUrl string) (*VideoParseInfo, error) {
	parsedUrl, err := url.Parse(itemUrl)
	if err != nil {
		return nil, err
	}

	itemId := parsedUrl.Query().Get("id")
	if itemId == "" {
		return nil, errors.New("item ID not found in URL")
	}

	apiURL := "https://h5api.m.goofish.com/h5/mtop.taobao.idle.pc.detail/1.0/"
	requestBody := fmt.Sprintf(`data={"itemId":"%s"}`, itemId)

	client := resty.New()
	cookie := "1=1;"
	res, err := client.R().
		SetHeader(HttpHeaderUserAgent, DefaultUserAgent).
		SetHeader(HttpHeaderContentType, "application/x-www-form-urlencoded").
		SetHeader(HttpHeaderCookie, cookie).
		SetBody(requestBody).
		Post(apiURL)
	if err != nil {
		return nil, err
	}

	data := gjson.ParseBytes(res.Body())
	if !strings.HasPrefix(data.Get("ret.0").String(), "SUCCESS") {
		return nil, fmt.Errorf("API error: %s", data.Get("ret.0").String())
	}

	itemDO := data.Get("data.itemDO")
	if !itemDO.Exists() {
		return nil, errors.New("item data not found in API response")
	}

	videoInfo := &VideoParseInfo{
		Title: itemDO.Get("title").String(),
	}

	sellerDO := data.Get("data.sellerDO")
	if sellerDO.Exists() {
		videoInfo.Author.Name = sellerDO.Get("nick").String()
		videoInfo.Author.Avatar = sellerDO.Get("portraitUrl").String()
	}

	// Extract images
	imageInfos := itemDO.Get("imageInfos").Array()
	images := make([]ImgInfo, 0, len(imageInfos))
	for _, img := range imageInfos {
		imgURL := img.Get("url").String()
		if imgURL != "" {
			images = append(images, ImgInfo{Url: imgURL})
		}
	}
	videoInfo.Images = images

	// If there are no images, try to get from shareData.contentParams.mainParams.images
	if len(videoInfo.Images) == 0 {
		shareImages := itemDO.Get("shareData.contentParams.mainParams.images").Array()
		for _, img := range shareImages {
			imgURL := img.Get("image").String()
			if imgURL != "" {
				images = append(images, ImgInfo{Url: imgURL})
			}
		}
		videoInfo.Images = images
	}

	if len(videoInfo.Images) == 0 {
		return nil, errors.New("no images found for this item")
	}

	return videoInfo, nil
}
