package pagination

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nicheinc/expect"
)

func Test_Decode(t *testing.T) {
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	validID := "0bc5b194-85ce-4e49-96be-0c91916e90ac"

	type testCase struct {
		input      string
		expected   Cursor
		errorCheck expect.ErrorCheck
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			actual, err := Decode(testCase.input)
			testCase.errorCheck(t, err)
			expect.Equal(t, actual, testCase.expected)
		})
	}

	run("NotBase64", testCase{input: "!!!", errorCheck: expect.ErrorIs(ErrInvalidCursor)})
	run("MissingSeparator", testCase{input: encode("2026-10-25T12:00:00Z"), errorCheck: expect.ErrorIs(ErrInvalidCursor)})
	run("BadTime", testCase{input: encode("yesterday|" + validID), errorCheck: expect.ErrorIs(ErrInvalidCursor)})
	run("BadUUID", testCase{input: encode("2026-10-25T12:00:00Z|nope"), errorCheck: expect.ErrorIs(ErrInvalidCursor)})
	run("Empty", testCase{errorCheck: expect.ErrorNil})
	cursor := Cursor{
		CreatedAt: time.Date(2026, 10, 25, 12, 0, 0, 123456789, time.UTC),
		UUID:      uuid.MustParse(validID),
	}
	run("RoundTrip", testCase{input: Encode(cursor), expected: cursor, errorCheck: expect.ErrorNil})
}

func Test_ParseLimit(t *testing.T) {
	type testCase struct {
		input      string
		expected   int
		errorCheck expect.ErrorCheck
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			actual, err := ParseLimit(testCase.input)
			testCase.errorCheck(t, err)
			expect.Equal(t, actual, testCase.expected)
		})
	}

	run("Zero", testCase{input: "0", errorCheck: expect.ErrorIs(ErrInvalidLimit)})
	run("Negative", testCase{input: "-1", errorCheck: expect.ErrorIs(ErrInvalidLimit)})
	run("TooLarge", testCase{input: "31", errorCheck: expect.ErrorIs(ErrInvalidLimit)})
	run("NotANumber", testCase{input: "abc", errorCheck: expect.ErrorIs(ErrInvalidLimit)})
	run("Empty", testCase{expected: DefaultLimit, errorCheck: expect.ErrorNil})
	run("Max", testCase{input: "30", expected: 30, errorCheck: expect.ErrorNil})
}

func Test_NewPage(t *testing.T) {
	type row struct {
		ID        uuid.UUID
		CreatedAt time.Time
	}
	cursorOf := func(r row) Cursor { return Cursor{CreatedAt: r.CreatedAt, UUID: r.ID} }
	base := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	rows := []row{
		{uuid.New(), base},
		{uuid.New(), base.Add(-time.Minute)},
		{uuid.New(), base.Add(-2 * time.Minute)},
	}

	t.Run("Empty", func(t *testing.T) {
		page := NewPage(nil, 2, cursorOf)
		expect.Equal(t, page.Items == nil, false)
		expect.Equal(t, len(page.Items), 0)
		expect.Equal(t, page.NextCursor, "")
	})
	t.Run("ExactlyLimit", func(t *testing.T) {
		page := NewPage(rows[:2], 2, cursorOf)
		expect.Equal(t, len(page.Items), 2)
		expect.Equal(t, page.NextCursor, "")
	})
	t.Run("MoreThanLimit", func(t *testing.T) {
		page := NewPage(rows, 2, cursorOf)
		expect.Equal(t, page.Items, rows[:2])
		expect.Equal(t, page.NextCursor, Encode(cursorOf(rows[1])))
	})
}
