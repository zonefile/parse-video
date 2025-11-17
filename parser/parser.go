package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/wujunwei928/parse-video/utils"
)

type KakaDownData struct {
	Author struct {
		Uid    string `json:"uid"`
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	} `json:"author"`
	Title    string   `json:"title"`
	VideoUrl string   `json:"video_url"`
	MusicUrl string   `json:"music_url"`
	CoverUrl string   `json:"cover_url"`
	Images   []string `json:"images"`
}

type KakaDownResponse struct {
	Code int           `json:"code"`
	Msg  string        `json:"msg"`
	Data *KakaDownData `json:"data"`
}

// ParseVideoShareUrlByRegexp 将分享链接信息, 进行正则表达式匹配到分享链接后, 再解析视频信息
func ParseVideoShareUrlByRegexp(shareMsg string) (*VideoParseInfo, error) {
	videoShareUrl, err := utils.RegexpMatchUrlFromString(shareMsg)
	if err != nil {
		return nil, err
	}

	return ParseVideoShareUrl(videoShareUrl)
}

// ParseVideoShareUrl 根据视频分享链接解析视频信息: 分享链接需是正常http链接
func ParseVideoShareUrl(shareUrl string) (*VideoParseInfo, error) {
	// 根据分享url判断source
	source := ""
	for itemSource, itemSourceInfo := range videoSourceInfoMapping {
		for _, itemUrlDomain := range itemSourceInfo.VideoShareUrlDomain {
			if strings.Contains(shareUrl, itemUrlDomain) {
				source = itemSource
				break
			}
		}
		if len(source) > 0 {
			break
		}
	}

	// 没有找到对应source
	if len(source) <= 0 {
		// fallback to kakadown.com
		apiURL := fmt.Sprintf("https://kakadown.com/video/share/url/parse?url=%s", url.QueryEscape(shareUrl))
		resp, err := http.Get(apiURL)
		if err != nil {
			return nil, fmt.Errorf("error making request to kakadown: %v", err)
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response body from kakadown: %v", err)
		}

		var kakaDownResponse KakaDownResponse
		if err := json.Unmarshal(body, &kakaDownResponse); err != nil {
			// try to unmarshal to VideoParseInfo for backward compatibility
			var videoInfo VideoParseInfo
			if err2 := json.Unmarshal(body, &videoInfo); err2 != nil {
				return nil, fmt.Errorf("error unmarshalling response from kakadown: %v", err)
			}
			return &videoInfo, nil
		}

		if kakaDownResponse.Data != nil {
			videoInfo := &VideoParseInfo{
				Title:    kakaDownResponse.Data.Title,
				VideoUrl: kakaDownResponse.Data.VideoUrl,
				MusicUrl: kakaDownResponse.Data.MusicUrl,
				CoverUrl: kakaDownResponse.Data.CoverUrl,
				Images:   make([]ImgInfo, len(kakaDownResponse.Data.Images)),
			}
			videoInfo.Author.Uid = kakaDownResponse.Data.Author.Uid
			videoInfo.Author.Name = kakaDownResponse.Data.Author.Name
			videoInfo.Author.Avatar = kakaDownResponse.Data.Author.Avatar

			for i, img := range kakaDownResponse.Data.Images {
				videoInfo.Images[i] = ImgInfo{Url: img}
			}
			return videoInfo, nil
		}

		return nil, errors.New("kakadown response data is nil")
	}

	// 没有对应的视频链接解析方法
	urlParser := videoSourceInfoMapping[source].VideoShareUrlParser
	if urlParser == nil {
		return nil, fmt.Errorf("source %s has no video share url parser", source)
	}

	return urlParser.parseShareUrl(shareUrl)
}

// ParseVideoId 根据视频id解析视频信息
func ParseVideoId(source, videoId string) (*VideoParseInfo, error) {
	if len(videoId) <= 0 || len(source) <= 0 {
		return nil, errors.New("video id or source is empty")
	}

	idParser := videoSourceInfoMapping[source].VideoIdParser
	if idParser == nil {
		return nil, fmt.Errorf("source %s has no video id parser", source)
	}

	return idParser.parseVideoID(videoId)
}

// BatchParseVideoId 根据视频id批量解析视频信息
func BatchParseVideoId(source string, videoIds []string) (map[string]BatchParseItem, error) {
	if len(videoIds) <= 0 || len(source) <= 0 {
		return nil, errors.New("videos id or source is empty")
	}

	idParser := videoSourceInfoMapping[source].VideoIdParser
	if idParser == nil {
		return nil, fmt.Errorf("source %s has no video id parser", source)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	parseMap := make(map[string]BatchParseItem, len(videoIds))
	for _, v := range videoIds {
		wg.Add(1)
		videoId := v
		go func(videoId string) {
			defer wg.Done()

			parseInfo, parseErr := ParseVideoId(source, videoId)
			mu.Lock()
			parseMap[videoId] = BatchParseItem{
				ParseInfo: parseInfo,
				Error:     parseErr,
			}
			mu.Unlock()
		}(videoId)
	}
	wg.Wait()

	return parseMap, nil
}
