package cli

import (
	"github.com/entireio/cli/cmd/entire/cli/api"
	"time"
)

// These CLI output types preserve the established JSON keys independently of
// the cell wire schema. Keep API structs at the HTTP boundary.

type trailReviewCommentJSON struct {
	ID                        string                           `json:"id"`
	TrailID                   string                           `json:"trailId"`
	RepositoryID              string                           `json:"repositoryId"`
	ReviewID                  string                           `json:"reviewId"`
	CodeVersionID             string                           `json:"codeVersionId"`
	ActorID                   string                           `json:"actorId"`
	Title                     *string                          `json:"title"`
	Body                      *string                          `json:"body"`
	Severity                  *string                          `json:"severity"`
	Confidence                *float64                         `json:"confidence"`
	Status                    string                           `json:"status"`
	StatusReason              *string                          `json:"statusReason"`
	StaleOutcome              string                           `json:"staleOutcome"`
	StaleCheckedAt            *time.Time                       `json:"staleCheckedAt"`
	StaleCheckedCodeVersionID *string                          `json:"staleCheckedCodeVersionId"`
	ClientID                  *string                          `json:"clientId"`
	ClientIDHash              *string                          `json:"clientIdHash"`
	CreatedAt                 time.Time                        `json:"createdAt"`
	UpdatedAt                 time.Time                        `json:"updatedAt"`
	Location                  trailReviewLocationJSON          `json:"location"`
	SuggestedChanges          []trailReviewSuggestedChangeJSON `json:"suggestedChanges,omitempty"`
	DiscussionID              *string                          `json:"discussionId,omitempty"`
	DiscussionMessageCount    int                              `json:"discussionMessageCount,omitempty"`
	OutgoingLinks             []trailReviewOutgoingLinkJSON    `json:"outgoingLinks,omitempty"`
}

func toTrailReviewCommentJSON(v api.TrailReviewComment) trailReviewCommentJSON {
	out := trailReviewCommentJSON{
		ID:                        v.ID,
		TrailID:                   v.TrailID,
		RepositoryID:              v.RepositoryID,
		ReviewID:                  v.ReviewID,
		CodeVersionID:             v.CodeVersionID,
		ActorID:                   v.ActorID,
		Title:                     v.Title,
		Body:                      v.Body,
		Severity:                  v.Severity,
		Confidence:                v.Confidence,
		Status:                    v.Status,
		StatusReason:              v.StatusReason,
		StaleOutcome:              v.StaleOutcome,
		StaleCheckedAt:            v.StaleCheckedAt,
		StaleCheckedCodeVersionID: v.StaleCheckedCodeVersionID,
		ClientID:                  v.ClientID,
		ClientIDHash:              v.ClientIDHash,
		CreatedAt:                 v.CreatedAt,
		UpdatedAt:                 v.UpdatedAt,
		DiscussionID:              v.DiscussionID,
		DiscussionMessageCount:    v.DiscussionMessageCount,
	}

	out.Location = toTrailReviewLocationJSON(v.Location)
	if v.SuggestedChanges != nil {
		out.SuggestedChanges = make([]trailReviewSuggestedChangeJSON, len(v.SuggestedChanges))
		for i := range v.SuggestedChanges {
			out.SuggestedChanges[i] = toTrailReviewSuggestedChangeJSON(v.SuggestedChanges[i])
		}
	}
	if v.OutgoingLinks != nil {
		out.OutgoingLinks = make([]trailReviewOutgoingLinkJSON, len(v.OutgoingLinks))
		for i := range v.OutgoingLinks {
			out.OutgoingLinks[i] = toTrailReviewOutgoingLinkJSON(v.OutgoingLinks[i])
		}
	}
	return out
}

type trailReviewLocationJSON struct {
	ID              string  `json:"id"`
	ReviewCommentID string  `json:"reviewCommentId"`
	CodeVersionID   string  `json:"codeVersionId"`
	Granularity     string  `json:"granularity"`
	FilePath        *string `json:"filePath"`
	StartLine       *int    `json:"startLine"`
	StartColumn     *int    `json:"startColumn"`
	EndLine         *int    `json:"endLine"`
	EndColumn       *int    `json:"endColumn"`
	SelectedText    *string `json:"selectedText"`
	NearbyText      *string `json:"nearbyText"`
	Language        *string `json:"language"`
}

func toTrailReviewLocationJSON(v api.TrailReviewLocation) trailReviewLocationJSON {
	return trailReviewLocationJSON(v)
}

type trailReviewSuggestedChangeJSON struct {
	ID                string    `json:"id"`
	ReviewCommentID   string    `json:"reviewCommentId"`
	CodeVersionID     string    `json:"codeVersionId"`
	ChangeType        string    `json:"changeType"`
	Patch             *string   `json:"patch"`
	Instruction       *string   `json:"instruction"`
	ExpectedFilePath  *string   `json:"expectedFilePath"`
	ExpectedFileHash  *string   `json:"expectedFileHash"`
	ExpectedStartLine *int      `json:"expectedStartLine"`
	ExpectedEndLine   *int      `json:"expectedEndLine"`
	ExpectedLines     *string   `json:"expectedLines"`
	CreatedBy         string    `json:"createdBy"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func toTrailReviewSuggestedChangeJSON(v api.TrailReviewSuggestedChange) trailReviewSuggestedChangeJSON {
	return trailReviewSuggestedChangeJSON(v)
}

type trailReviewOutgoingLinkJSON struct {
	SourceCommentID string `json:"sourceCommentId"`
	TargetCommentID string `json:"targetCommentId"`
	LinkType        string `json:"linkType"`
}

func toTrailReviewOutgoingLinkJSON(v api.TrailReviewOutgoingLink) trailReviewOutgoingLinkJSON {
	return trailReviewOutgoingLinkJSON(v)
}

func toTrailReviewCommentsJSON(comments []api.TrailReviewComment) []trailReviewCommentJSON {
	if comments == nil {
		return nil
	}
	out := make([]trailReviewCommentJSON, len(comments))
	for i := range comments {
		out[i] = toTrailReviewCommentJSON(comments[i])
	}
	return out
}
