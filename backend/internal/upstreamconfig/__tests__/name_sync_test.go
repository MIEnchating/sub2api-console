package upstreamconfig_test

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
	"testing"
)

type nameTaskQueue struct {
	hosts []string
	fail  bool
}

func (*nameTaskQueue) EnqueueHostAccountRateSync(context.Context, string, string) (string, error) {
	return "", nil
}
func (*nameTaskQueue) EnqueueHostAccountBaseURLSync(context.Context, string, string) (string, error) {
	return "", nil
}
func (q *nameTaskQueue) EnqueueHostAccountNameSync(_ context.Context, host, _ string) (string, error) {
	q.hosts = append(q.hosts, host)
	if q.fail {
		return "", errors.New("任务存储不可用")
	}
	return "name-task", nil
}

func TestManualNameSaveQueuesAccountUpdateAndReportsQueueFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "queue-failure"}[fail], func(t *testing.T) {
			f := newAddressFixture(t, "https://"+addressSourceHost)
			queue := &nameTaskQueue{fail: fail}
			service := upstreamconfig.New(f.repository, f.private, f.verifier, queue)
			name, mode := "自定义前缀", "manual"
			f.input.Name, f.input.NameMode = &name, &mode
			saved, err := service.Update(t.Context(), addressSourceHost, f.input, "test")
			if err != nil || saved.Name != name || saved.NameMode != mode {
				t.Fatalf("saved=%+v err=%v", saved, err)
			}
			if len(queue.hosts) != 1 || queue.hosts[0] != addressSourceHost {
				t.Fatalf("queued hosts=%v", queue.hosts)
			}
			if fail {
				if saved.NameSyncError == nil || saved.NameSyncTaskID != nil {
					t.Fatalf("queue failure hidden: %+v", saved)
				}
			} else if saved.NameSyncTaskID == nil || *saved.NameSyncTaskID != "name-task" {
				t.Fatalf("task missing: %+v", saved)
			}
			if _, err := service.Update(t.Context(), addressSourceHost, f.input, "test"); err != nil {
				t.Fatal(err)
			}
			if len(queue.hosts) != 1 {
				t.Fatal("unchanged name queued another task")
			}
		})
	}
}
