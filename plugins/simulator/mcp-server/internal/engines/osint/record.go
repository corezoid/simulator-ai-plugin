package osint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
)

// Buckets a collection can be recorded in.
const (
	BucketDay   = "day"
	BucketWeek  = "week"
	BucketMonth = "month"

	maxRecordEntries = 5000
	activityCurrency = "units"
)

// RecordResult says what recording a collection wrote.
type RecordResult struct {
	Actor    string           `json:"actorId"`
	Bucket   string           `json:"bucket"`
	From     string           `json:"from,omitempty"`
	To       string           `json:"to,omitempty"`
	Accounts []RecordedSeries `json:"accounts"`
	Skipped  string           `json:"skipped,omitempty"`
}

// RecordedSeries is one account a collection was recorded on.
type RecordedSeries struct {
	Name     string  `json:"accountName"`
	Currency string  `json:"currencyName"`
	Account  string  `json:"accountId"`
	Total    float64 `json:"total"`
	Entries  Tally   `json:"entries"`
}

// bucketStart is the first moment of the bucket a time falls in (UTC; weeks
// start on Monday).
func bucketStart(t time.Time, bucket string) time.Time {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	switch bucket {
	case BucketWeek:
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	case BucketMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return day
}

func bucketEnd(start time.Time, bucket string) time.Time {
	switch bucket {
	case BucketWeek:
		return start.AddDate(0, 0, 7)
	case BucketMonth:
		return start.AddDate(0, 1, 0)
	}
	return start.AddDate(0, 0, 1)
}

// series adds a collection up per bucket: the number of items, and the sum of
// each metric. Only buckets that had ended when the collection was made are
// kept: a week still running would be recorded short and never corrected.
func series(c *Collection, bucket string) (map[string]map[time.Time]float64, time.Time, time.Time) {
	retrieved, err := time.Parse(time.RFC3339, c.Retrieved)
	if err != nil {
		retrieved = time.Now().UTC()
	}
	out := map[string]map[time.Time]float64{}
	add := func(name string, at time.Time, v float64) {
		if out[name] == nil {
			out[name] = map[time.Time]float64{}
		}
		out[name][at] += v
	}
	var first, last time.Time
	for _, it := range c.Items {
		when, ok := itemTime(it.Date)
		if !ok {
			continue
		}
		start := bucketStart(when, bucket)
		if bucketEnd(start, bucket).After(retrieved) {
			continue
		}
		if first.IsZero() || start.Before(first) {
			first = start
		}
		if start.After(last) {
			last = start
		}
		add("items", start, 1)
		for name, v := range it.Metrics {
			if safeName.MatchString(name) && name != "items" {
				add(name, start, v)
			}
		}
	}
	// A collection cut at its limit starts part-way through its earliest
	// bucket: that bucket is short too, and is left out.
	if c.Truncated && !first.IsZero() && first.Before(last) {
		for _, s := range out {
			delete(s, first)
		}
		first = time.Time{}
		for _, s := range out {
			for at := range s {
				if first.IsZero() || at.Before(first) {
					first = at
				}
			}
		}
	}
	return out, first, last
}

// RecordActivity writes a collection onto an actor as accounts: one for the
// number of items and one for each metric, with an entry per bucket dated at
// the bucket's start. With the dates in place the accounts chart over time and
// can carry triggers. Entries are found again by their ref, so recording the
// same collection twice, or a later one that overlaps it, adds only what is
// new.
func RecordActivity(ctx context.Context, workspaceID, actorID string, c *Collection, bucket string) (*RecordResult, error) {
	if bucket == "" {
		bucket = BucketWeek
	}
	if bucket != BucketDay && bucket != BucketWeek && bucket != BucketMonth {
		return nil, fmt.Errorf("bucket %q is not one of day, week, month", bucket)
	}
	b := &builder{ctx: ctx, base: ecore.BuildBaseURLForContext(ctx), ws: ecore.Seg(workspaceID), res: &BuildResult{}}
	res := &RecordResult{Actor: actorID, Bucket: bucket, Accounts: []RecordedSeries{}}
	data, first, last := series(c, bucket)
	if len(data) == 0 {
		res.Skipped = "no dated item falls in a bucket that had ended when the collection was made"
		return res, nil
	}
	res.From, res.To = first.Format("2006-01-02"), last.Format("2006-01-02")
	entries := 0
	for _, s := range data {
		entries += len(s)
	}
	if entries > maxRecordEntries {
		return nil, fmt.Errorf("%d entries to record; at most %d — use a wider bucket", entries, maxRecordEntries)
	}
	label := c.Source + " " + c.Target
	for _, name := range sortedKeys(data) {
		accountName := "Activity · " + name
		var pair struct {
			AccountName struct {
				ID string `json:"id"`
			} `json:"accountName"`
			Currency struct {
				ID int `json:"id"`
			} `json:"currency"`
		}
		body := map[string]any{"accountName": accountName, "currencyName": activityCurrency, "precision": 0}
		if err := b.send(http.MethodPost, "/accounts/pair/"+b.ws, body, &pair); err != nil {
			return res, fmt.Errorf("account pair %s: %w", accountName, err)
		}
		if err := b.send(http.MethodPost, "/accounts/"+ecore.Seg(actorID)+"?ignoreIfExist=true",
			map[string]any{"nameId": pair.AccountName.ID, "currencyId": pair.Currency.ID}, nil); err != nil {
			return res, fmt.Errorf("account %s: %w", accountName, err)
		}
		var rows []struct {
			ID         string `json:"id"`
			NameID     string `json:"nameId"`
			CurrencyID int    `json:"currencyId"`
			IncomeType string `json:"incomeType"`
		}
		if err := b.get("/accounts/"+ecore.Seg(actorID)+"?"+url.Values{"limit": {"100"}, "query": {accountName}}.Encode(), &rows); err != nil {
			return res, fmt.Errorf("read account %s: %w", accountName, err)
		}
		credit := ""
		for _, r := range rows {
			if r.NameID == pair.AccountName.ID && r.CurrencyID == pair.Currency.ID && r.IncomeType == "credit" {
				credit = r.ID
			}
		}
		if credit == "" {
			return res, fmt.Errorf("account %s: Simulator did not return its credit side", accountName)
		}
		rec := RecordedSeries{Name: accountName, Currency: activityCurrency, Account: credit}
		starts := make([]time.Time, 0, len(data[name]))
		for at := range data[name] {
			starts = append(starts, at)
		}
		sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
		for _, at := range starts {
			v := data[name][at]
			rec.Total += v
			ref := fmt.Sprintf("osint:activity:%s:%s:%s:%s:%s", c.Source, slug(c.Target), name, bucket, at.Format("2006-01-02"))
			err := b.get("/transactions/ref/"+ecore.Seg(credit)+"/"+ecore.Seg(ref), nil)
			if err == nil {
				rec.Entries.Existing++
				continue
			}
			if !notFound(err) {
				return res, fmt.Errorf("look up entry %s: %w", ref, err)
			}
			entry := map[string]any{"amount": json.Number(strconv.FormatFloat(v, 'f', -1, 64)), "ref": ref, "originalDate": at.UnixMilli(),
				"comment": fmt.Sprintf("%s · %s of %s · %s", name, bucket, at.Format("2006-01-02"), label)}
			if err := b.send(http.MethodPost, "/transactions/"+ecore.Seg(credit), entry, nil); err != nil {
				return res, fmt.Errorf("entry %s: %w", ref, err)
			}
			rec.Entries.Created++
		}
		res.Accounts = append(res.Accounts, rec)
	}
	return res, nil
}
