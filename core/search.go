package core

import (
	"net/textproto"
	"time"

	eimap "github.com/emersion/go-imap"
)

// To match criteria message should match each field
// This structure is subset of go-imap.SearchCriteria.
type SearchCriteria struct {
	// Sent *before* (exclusive) specific date (time is ignored).
	Before time.Time
	// Sent *after* (exclusive) specific date (time is ignored).
	After time.Time
	// (NOT IMPLEMENTED) Sent on specific date (time is ignored). Should not be
	// used together with Before or After.
	On time.Time
	// Matched if From header contains specified substring.
	From string
	// Matched if subject or body contains specified substring.
	Text string
	// Message in any of listed directories. Defaults to full search.
	Dirs []string
}

func (sc SearchCriteria) toGoImap() eimap.SearchCriteria {
	res := eimap.SearchCriteria{}
	res.SentSince = sc.After
	res.SentBefore = sc.Before
	res.Header = make(textproto.MIMEHeader)
	if sc.From != "" {
		res.Header.Set("From", sc.From)
	}
	if sc.Text != "" {
		res.Or = [][2]*eimap.SearchCriteria{
			[2]*eimap.SearchCriteria{
				&eimap.SearchCriteria{
					Header: textproto.MIMEHeader{
						"Subject": []string{sc.Text},
					},
				},
				&eimap.SearchCriteria{
					Body: []string{sc.Text},
				},
			},
		}
	}
	return res
}

type SearchResult struct {
	Dir string
	Uid uint32
}

// Search for messages in directory matching specific criteria.
func (c *Client) Search(accountId string, criteria SearchCriteria) ([]SearchResult, error) {
	// IMAP supports only per-dir searches, so we emulate multi-dir search by
	// searching in each directory and joining results together.
	dirsToCheck := criteria.Dirs
	if dirsToCheck == nil {
		allDirs, err := c.GetDirs(accountId)
		if err != nil {
			return nil, err
		}
		dirsToCheck = allDirs.List()
	}

	res := []SearchResult{}

	for _, dir := range dirsToCheck {
		var matches []uint32
		var err error
		for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
			matches, err = c.imapConns[accountId].Search(dir, criteria.toGoImap())
			if err == nil || !connectionError(err) {
				break
			}
			if err := c.connectToServer(accountId); err != nil {
				return nil, err
			}
		}
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			res = append(res, SearchResult{dir, match})
		}
	}
	return res, nil
}
