package state

import (
	"errors"
	"fmt"

	"github.com/juanfont/headscale-v2/internal/types"
)

var (
	ErrNodeMarkedTaggedButHasNoTags = errors.New("node marked as tagged but has no tags")
	ErrNodeHasNeitherUserNorTags    = errors.New("node has neither user nor tags - must be owned by user or tagged")
	ErrRequestedTagsInvalidOrNotPermitted = errors.New("requested tags")
	ErrTaggedNodeHasUser            = errors.New("tagged node must not have user_id set")
)

// validateNodeOwnership ensures proper node ownership model.
func validateNodeOwnership(node *types.Node) error {
	if node.IsTagged() {
		if len(node.Tags) == 0 {
			return fmt.Errorf("%w: %q", ErrNodeMarkedTaggedButHasNoTags, node.Hostname)
		}
		if node.UserID != nil {
			return fmt.Errorf("%w: %q", ErrTaggedNodeHasUser, node.Hostname)
		}
		return nil
	}

	if node.UserID == nil {
		return fmt.Errorf("%w: %q", ErrNodeHasNeitherUserNorTags, node.Hostname)
	}

	return nil
}