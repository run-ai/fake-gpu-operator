package config

import (
	"fmt"
	"os"
)

func ValidateConfig(envVars []string) {
	err := validateEnvs(envVars)
	if err != nil {
		panic(err.Error())
	}
}

func validateEnvs(envVars []string) error {
	for _, env := range envVars {
		if os.Getenv(env) == "" {
			return fmt.Errorf("%s is not set", env)
		}
	}
	return nil
}
