package lfs

import "time"

// Git LFS Batch API types per https://github.com/git-lfs/git-lfs/blob/main/docs/api/batch.md

type BatchRequest struct {
	Operation string       `json:"operation"`
	Transfers []string     `json:"transfers,omitempty"`
	Ref       *Ref         `json:"ref,omitempty"`
	Objects   []ObjectSpec `json:"objects"`
}

type BatchResponse struct {
	Transfer string           `json:"transfer,omitempty"`
	Objects  []ObjectResponse `json:"objects"`
}

type Ref struct {
	Name string `json:"name"`
}

type ObjectSpec struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

type ObjectResponse struct {
	OID           string            `json:"oid"`
	Size          int64             `json:"size"`
	Authenticated *bool             `json:"authenticated,omitempty"`
	Actions       map[string]Action `json:"actions,omitempty"`
	Error         *ObjectError      `json:"error,omitempty"`
}

type Action struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
}

type ObjectError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type LFSError struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
}

type VerifyRequest struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}
