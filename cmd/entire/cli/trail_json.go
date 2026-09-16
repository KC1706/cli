package cli

import (
	"github.com/entireio/cli/cmd/entire/cli/api"
	"github.com/entireio/cli/cmd/entire/cli/trail"
	"time"
)

// These CLI output types preserve the established JSON keys independently of
// the cell wire schema. Keep API structs at the HTTP boundary.

type trailResourceJSON struct {
	ID                 string                 `json:"id,omitempty"`
	Number             int                    `json:"number,omitempty"`
	URL                string                 `json:"url,omitempty"`
	Branch             string                 `json:"branch"`
	OriginalBranch     string                 `json:"originalBranch,omitempty"`
	Base               string                 `json:"base"`
	Title              string                 `json:"title"`
	Body               string                 `json:"body,omitempty"`
	Status             string                 `json:"status"`
	Phase              string                 `json:"phase,omitempty"`
	Author             *trail.Author          `json:"author"`
	Assignees          []string               `json:"assignees"`
	Labels             []string               `json:"labels,omitempty"`
	Priority           string                 `json:"priority,omitempty"`
	Type               string                 `json:"type,omitempty"`
	Reviewers          []trail.Reviewer       `json:"reviewers,omitempty"`
	RequestedReviewers []string               `json:"requestedReviewers,omitempty"`
	CreatedAt          time.Time              `json:"createdAt"`
	UpdatedAt          time.Time              `json:"updatedAt"`
	MergedAt           *time.Time             `json:"mergedAt,omitempty"`
	CommentCount       int                    `json:"commentCount,omitempty"`
	UnresolvedCount    int                    `json:"unresolvedCount,omitempty"`
	CheckpointCount    int                    `json:"checkpointCount,omitempty"`
	CommitsAhead       int                    `json:"commitsAhead,omitempty"`
	BodyDocument       *trailBodyDocumentJSON `json:"bodyDocument,omitempty"`
}

func toTrailResourceJSON(v api.TrailResource) trailResourceJSON {
	out := trailResourceJSON{
		ID:                 v.ID,
		Number:             v.Number,
		URL:                v.URL,
		Branch:             v.Branch,
		OriginalBranch:     v.OriginalBranch,
		Base:               v.Base,
		Title:              v.Title,
		Body:               v.Body,
		Status:             v.Status,
		Phase:              v.Phase,
		Author:             v.Author,
		Assignees:          v.Assignees,
		Labels:             v.Labels,
		Priority:           v.Priority,
		Type:               v.Type,
		Reviewers:          v.Reviewers,
		RequestedReviewers: v.RequestedReviewers,
		CreatedAt:          v.CreatedAt,
		UpdatedAt:          v.UpdatedAt,
		MergedAt:           v.MergedAt,
		CommentCount:       v.CommentCount,
		UnresolvedCount:    v.UnresolvedCount,
		CheckpointCount:    v.CheckpointCount,
		CommitsAhead:       v.CommitsAhead,
	}

	if v.BodyDocument != nil {
		value := toTrailBodyDocumentJSON(*v.BodyDocument)
		out.BodyDocument = &value
	}
	return out
}

type trailBodyDocumentJSON struct {
	TextSnapshot string `json:"textSnapshot"`
	ETag         string `json:"etag,omitempty"`
}

func toTrailBodyDocumentJSON(v api.TrailBodyDocument) trailBodyDocumentJSON {
	return trailBodyDocumentJSON(v)
}

type trailApprovalsResponseJSON struct {
	Approvals []trailApprovalJSON `json:"approvals"`
}

func toTrailApprovalsResponseJSON(v api.TrailApprovalsResponse) trailApprovalsResponseJSON {
	out := trailApprovalsResponseJSON{}

	if v.Approvals != nil {
		out.Approvals = make([]trailApprovalJSON, len(v.Approvals))
		for i := range v.Approvals {
			out.Approvals[i] = toTrailApprovalJSON(v.Approvals[i])
		}
	}
	return out
}

type trailApprovalJSON struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Event     string    `json:"event"`
	Body      string    `json:"body,omitempty"`
	CommitSHA string    `json:"commitSha,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func toTrailApprovalJSON(v api.TrailApproval) trailApprovalJSON { return trailApprovalJSON(v) }

type trailDiscussionsResponseJSON struct {
	Items         []trailDiscussionSummaryJSON `json:"items"`
	NextPageToken *string                      `json:"nextPageToken,omitempty"`
	EventCursor   string                       `json:"eventCursor"`
}

func toTrailDiscussionsResponseJSON(v api.TrailDiscussionsResponse) trailDiscussionsResponseJSON {
	out := trailDiscussionsResponseJSON{
		NextPageToken: v.NextCursor,
		EventCursor:   v.EventCursor,
	}

	if v.Items != nil {
		out.Items = make([]trailDiscussionSummaryJSON, len(v.Items))
		for i := range v.Items {
			out.Items[i] = toTrailDiscussionSummaryJSON(v.Items[i])
		}
	}
	return out
}

type trailDiscussionSummaryJSON struct {
	ID                string                           `json:"id"`
	TrailID           string                           `json:"trailId"`
	Kind              string                           `json:"kind"` // "discussion" | "code_review"
	Title             string                           `json:"title"`
	ReviewCommentID   *string                          `json:"reviewCommentId"`
	Resolved          bool                             `json:"resolved"`
	ResolvedBy        *string                          `json:"resolvedBy"` // actor UUID
	ResolvedAt        *time.Time                       `json:"resolvedAt"`
	CreatedBy         *string                          `json:"createdBy"` // actor UUID
	CreatedAt         time.Time                        `json:"createdAt"`
	UpdatedAt         time.Time                        `json:"updatedAt"`
	LastMessageAt     *time.Time                       `json:"lastMessageAt"`
	LastMessageAuthor *string                          `json:"lastMessageAuthor"` // GitHub login
	MessageCount      int                              `json:"messageCount"`
	Participants      []trailDiscussionParticipantJSON `json:"participants"`
}

func toTrailDiscussionSummaryJSON(v api.TrailDiscussionSummary) trailDiscussionSummaryJSON {
	out := trailDiscussionSummaryJSON{
		ID:                v.ID,
		TrailID:           v.TrailID,
		Kind:              v.Kind,
		Title:             v.Title,
		ReviewCommentID:   v.ReviewCommentID,
		Resolved:          v.Resolved,
		ResolvedBy:        v.ResolvedBy,
		ResolvedAt:        v.ResolvedAt,
		CreatedBy:         v.CreatedBy,
		CreatedAt:         v.CreatedAt,
		UpdatedAt:         v.UpdatedAt,
		LastMessageAt:     v.LastMessageAt,
		LastMessageAuthor: v.LastMessageAuthor,
		MessageCount:      v.MessageCount,
	}

	if v.Participants != nil {
		out.Participants = make([]trailDiscussionParticipantJSON, len(v.Participants))
		for i := range v.Participants {
			out.Participants[i] = toTrailDiscussionParticipantJSON(v.Participants[i])
		}
	}
	return out
}

type trailDiscussionParticipantJSON struct {
	Login string `json:"login"`
}

func toTrailDiscussionParticipantJSON(v api.TrailDiscussionParticipant) trailDiscussionParticipantJSON {
	return trailDiscussionParticipantJSON(v)
}

type trailDiscussionDetailResponseJSON struct {
	Discussion  trailDiscussionSummaryJSON   `json:"discussion"`
	Messages    []trailDiscussionMessageJSON `json:"messages"`
	EventCursor string                       `json:"eventCursor"`
}

func toTrailDiscussionDetailResponseJSON(v api.TrailDiscussionDetailResponse) trailDiscussionDetailResponseJSON {
	out := trailDiscussionDetailResponseJSON{
		EventCursor: v.EventCursor,
	}

	out.Discussion = toTrailDiscussionSummaryJSON(v.Discussion)
	if v.Messages != nil {
		out.Messages = make([]trailDiscussionMessageJSON, len(v.Messages))
		for i := range v.Messages {
			out.Messages[i] = toTrailDiscussionMessageJSON(v.Messages[i])
		}
	}
	return out
}

type trailDiscussionMessageJSON struct {
	ID        string                     `json:"id"`
	Author    string                     `json:"author"` // GitHub login
	CreatedAt time.Time                  `json:"createdAt"`
	Body      string                     `json:"body"`
	Replies   []trailDiscussionReplyJSON `json:"replies"`
}

func toTrailDiscussionMessageJSON(v api.TrailDiscussionMessage) trailDiscussionMessageJSON {
	out := trailDiscussionMessageJSON{
		ID:        v.ID,
		Author:    v.Author,
		CreatedAt: v.CreatedAt,
		Body:      v.Body,
	}

	if v.Replies != nil {
		out.Replies = make([]trailDiscussionReplyJSON, len(v.Replies))
		for i := range v.Replies {
			out.Replies[i] = toTrailDiscussionReplyJSON(v.Replies[i])
		}
	}
	return out
}

type trailDiscussionReplyJSON struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"` // GitHub login
	CreatedAt time.Time `json:"createdAt"`
	Body      string    `json:"body"`
}

func toTrailDiscussionReplyJSON(v api.TrailDiscussionReply) trailDiscussionReplyJSON {
	return trailDiscussionReplyJSON(v)
}

type trailDiscussionCreateResponseJSON struct {
	Discussion trailDiscussionSummaryJSON  `json:"discussion"`
	Message    *trailDiscussionMessageJSON `json:"message"`
}

func toTrailDiscussionCreateResponseJSON(v api.TrailDiscussionCreateResponse) trailDiscussionCreateResponseJSON {
	out := trailDiscussionCreateResponseJSON{}

	out.Discussion = toTrailDiscussionSummaryJSON(v.Discussion)
	if v.Message != nil {
		value := toTrailDiscussionMessageJSON(*v.Message)
		out.Message = &value
	}
	return out
}
