package util

import "github.com/google/uuid"

func StringToUUID(s string) string {
	return uuid.NewSHA1(uuid.Nil, []byte(s)).String()
}
