package util

import "log"

func LogErrorIfExist(err error, message string) {
	if err != nil {
		log.Printf("%s: %v", message, err)
	}
}
