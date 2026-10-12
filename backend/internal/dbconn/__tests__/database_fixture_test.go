package dbconn_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"testing"
)

func databaseConfigs(t *testing.T) []dbconn.Config { return dbtest.Configs(t) }
