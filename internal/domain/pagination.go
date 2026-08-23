package domain

import (
	"strings"
	"time"
)

type PageRequest struct {
	Page     int
	PageSize int
	Sort     string
	Desc     bool
}

func (p PageRequest) Normalize(allowedSorts map[string]bool, fallback string) PageRequest {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	p.Sort = strings.TrimSpace(strings.ToLower(p.Sort))
	if !allowedSorts[p.Sort] {
		p.Sort = fallback
	}
	return p
}

func (p PageRequest) Offset() int { return (p.Page - 1) * p.PageSize }

type CaseFilter struct {
	Statuses     []CaseStatus
	ForestSiteID *ID
	ZoneID       *ID
	OwnerID      *ID
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
	Search       string
}

func (f CaseFilter) Clone() CaseFilter {
	copyFilter := f
	copyFilter.Statuses = append([]CaseStatus(nil), f.Statuses...)
	if f.ForestSiteID != nil {
		value := *f.ForestSiteID
		copyFilter.ForestSiteID = &value
	}
	if f.ZoneID != nil {
		value := *f.ZoneID
		copyFilter.ZoneID = &value
	}
	if f.OwnerID != nil {
		value := *f.OwnerID
		copyFilter.OwnerID = &value
	}
	if f.CreatedFrom != nil {
		value := *f.CreatedFrom
		copyFilter.CreatedFrom = &value
	}
	if f.CreatedTo != nil {
		value := *f.CreatedTo
		copyFilter.CreatedTo = &value
	}
	return copyFilter
}

type Page[T any] struct {
	Items    []T `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}
