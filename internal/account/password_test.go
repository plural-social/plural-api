package account

import (
	"testing"

	"github.com/nicheinc/expect"
)

func Test_VerifyPassword(t *testing.T) {
	hash := expect.Must(HashPassword("correct horse battery staple"))(t)

	type testCase struct {
		password    string
		encodedHash string
		expected    bool
		errorCheck  expect.ErrorCheck
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			actual, err := VerifyPassword(testCase.password, testCase.encodedHash)
			testCase.errorCheck(t, err)
			expect.Equal(t, actual, testCase.expected)
		})
	}

	run("MalformedHash", testCase{
		password:    "correct horse battery staple",
		encodedHash: "not-a-hash",
		errorCheck:  expect.ErrorIs(ErrInvalidPasswordHash),
	})
	run("WrongAlgorithm", testCase{
		password:    "correct horse battery staple",
		encodedHash: "$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$a2V5",
		errorCheck:  expect.ErrorIs(ErrInvalidPasswordHash),
	})
	run("WrongPassword", testCase{
		password:    "wrong password",
		encodedHash: hash,
		expected:    false,
		errorCheck:  expect.ErrorNil,
	})
	run("CorrectPassword", testCase{
		password:    "correct horse battery staple",
		encodedHash: hash,
		expected:    true,
		errorCheck:  expect.ErrorNil,
	})
}
