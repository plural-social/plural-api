package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"

	"github.com/plural-social/plural-api/internal/account"
	"github.com/plural-social/plural-api/internal/media"
	"github.com/plural-social/plural-api/internal/moderation"
	"github.com/plural-social/plural-api/internal/pagination"
	"github.com/plural-social/plural-api/internal/social"
)

func Test_Store_blockVisibility(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	a, b, c := insertTestAccount(t, store), insertTestAccount(t, store), insertTestAccount(t, store)

	expect.ErrorNil(t, store.Follow(ctx, a.UUID, b.UUID))
	expect.ErrorNil(t, store.Follow(ctx, b.UUID, a.UUID))
	aPost := insertTestPost(t, store, a.UUID, media.StatusReady)
	bPost := insertTestPost(t, store, b.UUID, media.StatusReady)
	cPost := insertTestPost(t, store, c.UUID, media.StatusReady)
	bComment, err := store.InsertComment(ctx, social.NewComment{PostUUID: cPost, AccountUUID: b.UUID, Body: "from b"})
	expect.ErrorNil(t, err)

	// canSee reports whether viewer can see post through every read path.
	canSee := func(t *testing.T, viewer *uuid.UUID, author account.Account, postUUID uuid.UUID) bool {
		t.Helper()
		_, getErr := store.GetPost(ctx, postUUID, viewer)
		explore, err := store.ListExploreFeed(ctx, viewer, pagination.Cursor{}, 1000)
		expect.ErrorNil(t, err)
		userPosts, err := store.ListUserPosts(ctx, author.UUID, viewer, pagination.Cursor{}, 30)
		expect.ErrorNil(t, err)
		inExplore, inUserPosts := len(postUUIDs(explore, postUUID)) == 1, len(postUUIDs(userPosts, postUUID)) == 1
		// All three paths must agree, or one of them leaks.
		expect.Equal(t, inExplore, getErr == nil)
		expect.Equal(t, inUserPosts, getErr == nil)
		return getErr == nil
	}
	inFollowing := func(t *testing.T, viewer uuid.UUID, postUUID uuid.UUID) bool {
		t.Helper()
		rows, err := store.ListFollowingFeed(ctx, viewer, pagination.Cursor{}, 1000)
		expect.ErrorNil(t, err)
		return len(postUUIDs(rows, postUUID)) == 1
	}
	commentVisible := func(t *testing.T, viewer *uuid.UUID) bool {
		t.Helper()
		list, err := store.ListComments(ctx, cPost, viewer, pagination.Cursor{}, 30)
		expect.ErrorNil(t, err)
		_, getErr := store.GetComment(ctx, bComment, viewer)
		expect.Equal(t, len(list) == 1, getErr == nil)
		return getErr == nil
	}

	expect.Equal(t, inFollowing(t, a.UUID, bPost), true)
	expect.ErrorNil(t, store.BlockAccount(ctx, a.UUID, b.UUID))

	t.Run("BlockerCannotSeeBlocked", func(t *testing.T) {
		expect.Equal(t, canSee(t, &a.UUID, b, bPost), false)
		expect.Equal(t, commentVisible(t, &a.UUID), false)
	})
	t.Run("BlockedCannotSeeBlocker", func(t *testing.T) {
		expect.Equal(t, canSee(t, &b.UUID, a, aPost), false)
	})
	t.Run("FollowsRemovedBothWays", func(t *testing.T) {
		expect.Equal(t, inFollowing(t, a.UUID, bPost), false)
		expect.Equal(t, inFollowing(t, b.UUID, aPost), false)
		profile, err := store.GetProfile(ctx, b.Username, &a.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, profile.IsFollowing, false)
		expect.Equal(t, profile.IsBlocked, true)
	})
	t.Run("OthersUnaffected", func(t *testing.T) {
		expect.Equal(t, canSee(t, nil, a, aPost), true)
		expect.Equal(t, canSee(t, nil, b, bPost), true)
		expect.Equal(t, canSee(t, &c.UUID, b, bPost), true)
		expect.Equal(t, commentVisible(t, nil), true)
		expect.Equal(t, commentVisible(t, &c.UUID), true)
	})
	t.Run("IsBlockedEitherWay", func(t *testing.T) {
		for _, pair := range [][2]uuid.UUID{{a.UUID, b.UUID}, {b.UUID, a.UUID}} {
			blocked, err := store.IsBlockedEitherWay(ctx, pair[0], pair[1])
			expect.ErrorNil(t, err)
			expect.Equal(t, blocked, true)
		}
		blocked, err := store.IsBlockedEitherWay(ctx, a.UUID, c.UUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, blocked, false)
	})
	t.Run("ListBlocks", func(t *testing.T) {
		blocks, err := store.ListBlocks(ctx, a.UUID, pagination.Cursor{}, 10)
		expect.ErrorNil(t, err)
		expect.Equal(t, len(blocks), 1)
		expect.Equal(t, blocks[0].UUID, b.UUID)
	})
	t.Run("UnblockRestores", func(t *testing.T) {
		expect.ErrorNil(t, store.UnblockAccount(ctx, a.UUID, b.UUID))
		expect.Equal(t, canSee(t, &a.UUID, b, bPost), true)
		expect.Equal(t, canSee(t, &b.UUID, a, aPost), true)
		expect.Equal(t, commentVisible(t, &a.UUID), true)
	})
}

func Test_Store_suspension(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	b, viewer := insertTestAccount(t, store), insertTestAccount(t, store)
	bPost := insertTestPost(t, store, b.UUID, media.StatusReady)
	otherPost := insertTestPost(t, store, viewer.UUID, media.StatusReady)
	bComment, err := store.InsertComment(ctx, social.NewComment{PostUUID: otherPost, AccountUUID: b.UUID, Body: "x"})
	expect.ErrorNil(t, err)

	_, tokenHash, err := account.NewSessionToken()
	expect.ErrorNil(t, err)
	expect.ErrorNil(t, store.InsertSession(ctx, account.NewSession{TokenHash: tokenHash, AccountUUID: b.UUID, ExpiresAt: time.Now().Add(time.Hour)}))

	expect.ErrorNil(t, store.SuspendAccount(ctx, b.UUID))

	t.Run("SessionsEnded", func(t *testing.T) {
		_, err := store.GetViewer(ctx, tokenHash)
		expect.ErrorIs(ErrSessionNotFound)(t, err)
	})
	t.Run("ContentHidden", func(t *testing.T) {
		for _, v := range []*uuid.UUID{nil, &viewer.UUID, &b.UUID} {
			_, err := store.GetPost(ctx, bPost, v)
			expect.ErrorIs(ErrPostNotFound)(t, err)
			_, err = store.GetComment(ctx, bComment, v)
			expect.ErrorIs(ErrCommentNotFound)(t, err)
		}
		explore, err := store.ListExploreFeed(ctx, nil, pagination.Cursor{}, 1000)
		expect.ErrorNil(t, err)
		expect.Equal(t, len(postUUIDs(explore, bPost)), 0)
	})
	t.Run("ProfileHidden", func(t *testing.T) {
		_, err := store.GetProfile(ctx, b.Username, nil)
		expect.ErrorIs(ErrAccountNotFound)(t, err)
		_, err = store.GetUserSummary(ctx, b.UUID)
		expect.ErrorIs(ErrAccountNotFound)(t, err)
	})
	t.Run("CredentialsShowSuspension", func(t *testing.T) {
		credentials, err := store.GetCredentials(ctx, b.Username)
		expect.ErrorNil(t, err)
		expect.Equal(t, credentials.SuspendedAt != nil, true)
	})
	t.Run("UnsuspendRestores", func(t *testing.T) {
		expect.ErrorNil(t, store.UnsuspendAccount(ctx, b.UUID))
		_, err := store.GetPost(ctx, bPost, nil)
		expect.ErrorNil(t, err)
		_, err = store.GetProfile(ctx, b.Username, nil)
		expect.ErrorNil(t, err)
		// Sessions deleted by the suspension stay deleted.
		_, err = store.GetViewer(ctx, tokenHash)
		expect.ErrorIs(ErrSessionNotFound)(t, err)
		expect.ErrorIs(ErrAccountNotFound)(t, store.UnsuspendAccount(ctx, uuid.New()))
	})
}

func Test_Store_reports(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	reporter, author, moderator := insertTestAccount(t, store), insertTestAccount(t, store), insertTestAccount(t, store)
	expect.ErrorNil(t, store.UpdateProfile(ctx, author.UUID, account.ProfileUpdate{Bio: new("author bio")}))

	m := insertTestMedia(t, store, author.UUID, media.StatusReady)
	postUUID := insertTestPostOn(t, store, author.UUID, m.UUID, "post caption")
	commentUUID, err := store.InsertComment(ctx, social.NewComment{PostUUID: postUUID, AccountUUID: author.UUID, Body: "comment body"})
	expect.ErrorNil(t, err)

	report := func(targetType string, target uuid.UUID) uuid.UUID {
		id, err := store.InsertReport(ctx, moderation.NewReport{
			ReporterUUID: reporter.UUID, TargetType: targetType, TargetUUID: target, Reason: "spam",
		})
		expect.ErrorNil(t, err)
		return id
	}
	postReport1, postReport2 := report("post", postUUID), report("post", postUUID)
	commentReport := report("comment", commentUUID)
	accountReport := report("account", author.UUID)

	t.Run("Previews", func(t *testing.T) {
		for id, expected := range map[uuid.UUID]moderation.TargetPreview{
			postReport1:   {Username: author.Username, Text: "post caption", ThumbnailKey: m.ThumbnailKey},
			commentReport: {Username: author.Username, Text: "comment body"},
			accountReport: {Username: author.Username, Text: "author bio"},
		} {
			got, err := store.GetReport(ctx, id)
			expect.ErrorNil(t, err)
			expect.Equal(t, *got.TargetPreview, expected)
			expect.Equal(t, got.Reporter.UUID, reporter.UUID)
			expect.Equal(t, got.Status, moderation.StatusOpen)
		}
	})
	t.Run("ListOpen", func(t *testing.T) {
		open, err := store.ListReports(ctx, moderation.StatusOpen, pagination.Cursor{}, 30)
		expect.ErrorNil(t, err)
		found := 0
		for _, r := range open {
			if r.Reporter.UUID == reporter.UUID {
				found++
			}
		}
		expect.Equal(t, found, 4)
	})
	t.Run("ResolveOnlyOne", func(t *testing.T) {
		expect.ErrorNil(t, store.ResolveReports(ctx, moderation.Resolution{
			TargetType: "post", TargetUUID: postUUID, ModeratorUUID: moderator.UUID,
			Status: moderation.StatusDismissed, OnlyReportUUID: &postReport1,
		}))
		first, err := store.GetReport(ctx, postReport1)
		expect.ErrorNil(t, err)
		expect.Equal(t, first.Status, moderation.StatusDismissed)
		expect.Equal(t, first.ResolvedAt != nil, true)
		second, err := store.GetReport(ctx, postReport2)
		expect.ErrorNil(t, err)
		expect.Equal(t, second.Status, moderation.StatusOpen)
	})
	t.Run("ResolveAllOnTarget", func(t *testing.T) {
		postReport3 := report("post", postUUID)
		expect.ErrorNil(t, store.ResolveReports(ctx, moderation.Resolution{
			TargetType: "post", TargetUUID: postUUID, ModeratorUUID: moderator.UUID,
			Status: moderation.StatusResolved, Note: "removed",
		}))
		for id, expected := range map[uuid.UUID]string{
			postReport1:   moderation.StatusDismissed, // already closed: untouched
			postReport2:   moderation.StatusResolved,
			postReport3:   moderation.StatusResolved,
			commentReport: moderation.StatusOpen, // other target: untouched
		} {
			got, err := store.GetReport(ctx, id)
			expect.ErrorNil(t, err)
			expect.Equal(t, got.Status, expected)
		}
	})
	t.Run("ContentAuthorIgnoresVisibility", func(t *testing.T) {
		_, err := store.RemovePost(ctx, postUUID)
		expect.ErrorNil(t, err)
		_, err = store.RemovePost(ctx, postUUID)
		expect.ErrorIs(ErrPostNotFound)(t, err)
		got, err := store.GetContentAuthor(ctx, "post", postUUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, got, author.UUID)
		got, err = store.GetContentAuthor(ctx, "comment", commentUUID)
		expect.ErrorNil(t, err)
		expect.Equal(t, got, author.UUID)
	})
}

func Test_Store_roles(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	user := insertTestAccount(t, store)
	_, err := store.db.ExecContext(ctx, `UPDATE account SET role = 'moderator' WHERE uuid = $1`, user.UUID)
	expect.ErrorNil(t, err)

	_, tokenHash, err := account.NewSessionToken()
	expect.ErrorNil(t, err)
	expect.ErrorNil(t, store.InsertSession(ctx, account.NewSession{TokenHash: tokenHash, AccountUUID: user.UUID, ExpiresAt: time.Now().Add(time.Hour)}))

	viewer, err := store.GetViewer(ctx, tokenHash)
	expect.ErrorNil(t, err)
	expect.Equal(t, viewer.Role, account.RoleModerator)
	role, err := store.GetAccountRole(ctx, user.UUID)
	expect.ErrorNil(t, err)
	expect.Equal(t, role, account.RoleModerator)
}
