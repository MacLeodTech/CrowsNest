package main

import (
	"os"
	"testing"

	"github.com/macleodtech/crowsnest/internal/auth"
)

func TestMain(m *testing.M) {
	auth.PasswordCost = 4 // bcrypt's minimum: hashing is the slow part of every test
	os.Exit(m.Run())
}
