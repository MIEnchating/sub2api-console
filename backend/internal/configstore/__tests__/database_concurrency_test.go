package configstore_test

import (
	"errors"
	"strconv"
	"testing"
)

func TestDatabaseContractConcurrentSettingsRetainEveryUpdate(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			for _, initial := range []string{"missing", "existing"} {
				t.Run(initial, func(t *testing.T) {
					keys := []string{"concurrent." + initial + ".first", "concurrent." + initial + ".second"}
					if initial == "existing" {
						for _, key := range keys {
							if err := store.SaveJSONSetting(t.Context(), key, "0"); err != nil {
								t.Fatal(err)
							}
						}
					}
					const writers = 16
					start, results := make(chan struct{}), make(chan error, writers)
					for i := range writers {
						go func() {
							<-start
							scope := keys
							if i%2 == 1 {
								scope = []string{keys[1], keys[0]}
							}
							results <- store.UpdateJSONSettings(t.Context(), scope, func(values map[string]string) (map[string]string, error) {
								if values[keys[0]] != values[keys[1]] {
									return nil, errors.New("inconsistent settings snapshot")
								}
								n := 0
								if values[keys[0]] != "" {
									var err error
									n, err = strconv.Atoi(values[keys[0]])
									if err != nil {
										return nil, err
									}
								}
								return map[string]string{keys[0]: strconv.Itoa(n + 1), keys[1]: strconv.Itoa(n + 1)}, nil
							})
						}()
					}
					close(start)
					for range writers {
						if err := <-results; err != nil {
							t.Error(err)
						}
					}
					for _, key := range keys {
						value, err := store.JSONSetting(t.Context(), key)
						if err != nil || value != strconv.Itoa(writers) {
							t.Errorf("lost update: %s=%q, error=%v", key, value, err)
						}
					}
				})
			}
		})
	}
}
