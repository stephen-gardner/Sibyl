package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

type (
	BatmanTime struct {
		time.Time
	}
	BatmanResult struct {
		Login            string `json:"login"`
		Project          string `json:"project"`
		Repo             string `json:"repo"`
		AuthorFile       string `json:"author"`
		Headers          string `json:"headers"`
		MatchedFunctions []struct {
			Cheater string `json:"cheater"`
			Name    string `json:"func"`
			Matches []struct {
				Login    string     `json:"login"`
				Date     BatmanTime `json:"date"`
				Name     string     `json:"func_name"`
				Filename string     `json:"filename"`
			} `json:"match"`
		} `json:"matches"`
	}
)

const (
	batmanError         = "error"
	batmanClean         = "clean"
	batmanEmpty         = "empty"
	batmanNotApplicable = "N/A"
	batmanTimeFormat    = "2 Jan 2006 15:04"
)

func (bt *BatmanTime) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(string(data), "\"")
	if raw == "null" {
		bt.Time = time.Time{}
		return nil
	}
	date, err := time.ParseInLocation(batmanTimeFormat, raw, time.Local)
	if err == nil {
		bt.Time = date
	}
	return err
}

func (res *BatmanResult) getSize() int {
	size := 0
	for _, function := range res.MatchedFunctions {
		size += len(function.Matches)
	}
	return size
}

func (res *BatmanResult) mapFunctionsToUsers() map[string][]string {
	matches := make(map[string][]string)
	for _, function := range res.MatchedFunctions {
		for _, match := range function.Matches {
			key := fmt.Sprintf("%s [%s]", match.Login, match.Date.Format(time.RFC822))
			if _, present := matches[key]; !present {
				matches[key] = make([]string, 0)
			}
			val := fmt.Sprintf("%s\t=\t%s\t<%s>", function.Name, match.Name, match.Filename)
			matches[key] = append(matches[key], val)
		}
	}
	return matches
}

// Returns index of matches, ordered by number of matches, then alphabetically by user login
func indexMatches(matches map[string][]string) []string {
	index := make([]string, len(matches))
	i := 0
	for key := range matches {
		index[i] = key
		i++
	}
	sort.Slice(index, func(i, j int) bool {
		k1, k2 := index[i], index[j]
		v1, v2 := matches[k1], matches[k2]
		if len(v1) == len(v2) {
			return strings.Compare(k1, k2) < 0
		}
		return len(v1) > len(v2)
	})
	return index
}

func (res *BatmanResult) getFormattedOutput() string {
	matches := res.mapFunctionsToUsers()
	breakdown := &strings.Builder{}
	// Keep track of where user headers will be inserted into aligned output
	breakdownLengths := make([]int, 0)
	tw := tabwriter.NewWriter(breakdown, 0, 1, 4, ' ', 0)
	users := indexMatches(matches)
	for i, key := range users {
		matchList := matches[key]
		users[i] = fmt.Sprint(key)
		breakdownLengths = append(breakdownLengths, len(matchList))
		for _, match := range matchList {
			_, _ = fmt.Fprintf(tw, "\t%s\n", match)
		}
	}
	_ = tw.Flush()
	sb := &strings.Builder{}
	breakdownArr := strings.Split(breakdown.String(), "\n")
	for i, source := range users {
		_, _ = fmt.Fprintf(sb, "%s\n", source)
		for j := 0; j < breakdownLengths[i]; j++ {
			_, _ = fmt.Fprintf(sb, "%s\n", breakdownArr[j])
		}
		breakdownArr = breakdownArr[breakdownLengths[i]:]
	}
	out := sb.String()
	return out[:len(out)-1]
}

// Sort matched functions alphabetically and remove duplicate data returned by Batman
func (res *BatmanResult) slimDown() {
	if len(res.MatchedFunctions) == 0 {
		return
	}
	sort.Slice(res.MatchedFunctions, func(i, j int) bool {
		return strings.Compare(res.MatchedFunctions[i].Name, res.MatchedFunctions[j].Name) < 0
	})
	i := 1
	for _, function := range res.MatchedFunctions {
		if reflect.DeepEqual(function, res.MatchedFunctions[i-1]) {
			continue
		}
		res.MatchedFunctions[i] = function
		i++
	}
	res.MatchedFunctions = res.MatchedFunctions[:i]
}

func runBatman(login, project, repoURL string) (status string, res *BatmanResult, err error) {
	if !strings.Contains(repoURL, config.CampusDomain) {
		return batmanNotApplicable, nil, nil
	}
	var URL *url.URL
	URL, err = url.Parse(config.BatmanEndpoint)
	if err != nil {
		return batmanError, nil, err
	}
	params := url.Values{}
	params.Set("login", login)
	params.Set("project", project)
	params.Set("repo", repoURL)
	URL.RawQuery = params.Encode()
	resp, err := http.Get(URL.String())
	if err != nil {
		return batmanError, nil, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		err = fmt.Errorf("batman error [response: %d] %s", resp.StatusCode, string(data))
		return batmanError, nil, err
	}
	if strings.Contains(string(data), "\"No cheaters detected\"") {
		return batmanClean, nil, nil
	}
	if strings.Contains(string(data), "\"Empty repo\"") {
		return batmanEmpty, nil, nil
	}
	res = &BatmanResult{}
	if err := json.Unmarshal(data, res); err != nil {
		err = fmt.Errorf("batman error: %s: %s", err.Error(), string(data))
		return batmanError, nil, err
	}
	res.slimDown()
	return fmt.Sprintf("%d matches found", res.getSize()), res, nil
}
