package media

import (
	"testing"
)

func TestQueueMaxWorkers(t *testing.T) {
	qm := NewQueueManager(3)
	if qm.GetMaxWorkers() != 3 {
		t.Errorf("expected maxWorkers=3, got %d", qm.GetMaxWorkers())
	}

	qm.SetMaxWorkers(5)
	if qm.GetMaxWorkers() != 5 {
		t.Errorf("expected maxWorkers=5, got %d", qm.GetMaxWorkers())
	}

	// Test boundary constraints
	qm.SetMaxWorkers(0)
	if qm.GetMaxWorkers() != 1 {
		t.Errorf("expected maxWorkers=1 for non-positive input, got %d", qm.GetMaxWorkers())
	}

	qm.SetMaxWorkers(100)
	if qm.GetMaxWorkers() != 16 {
		t.Errorf("expected maxWorkers=16 for excessive input, got %d", qm.GetMaxWorkers())
	}
}

func TestParseProgressLineAria2AndYtDlp(t *testing.T) {
	qm := NewQueueManager(4)

	ariaTask := &DownloadTask{ID: "t-aria", Status: StatusDownloading}
	qm.parseProgressLine(ariaTask, "[#2089b0 128MiB/512MiB(25%) CN:16 DL:48.5MiB ETA:8s]")
	if ariaTask.Percent != 25 {
		t.Errorf("expected aria2c Percent=25, got %v", ariaTask.Percent)
	}
	if ariaTask.Speed != "48.5MiB/s" {
		t.Errorf("expected aria2c Speed=\"48.5MiB/s\", got %q", ariaTask.Speed)
	}
	if ariaTask.ETA != "8s" {
		t.Errorf("expected aria2c ETA=\"8s\", got %q", ariaTask.ETA)
	}
	if ariaTask.HumanSize != "512MiB" {
		t.Errorf("expected aria2c HumanSize=\"512MiB\", got %q", ariaTask.HumanSize)
	}

	ytTask := &DownloadTask{ID: "t-yt", Status: StatusDownloading}
	qm.parseProgressLine(ytTask, "70.5%|15.2MiB/s|00:12|1000|2000")
	if ytTask.Percent != 70.5 {
		t.Errorf("expected yt-dlp Percent=70.5, got %v", ytTask.Percent)
	}
	if ytTask.Speed != "15.2MiB/s" {
		t.Errorf("expected yt-dlp Speed=\"15.2MiB/s\", got %q", ytTask.Speed)
	}
	if ytTask.ETA != "00:12" {
		t.Errorf("expected yt-dlp ETA=\"00:12\", got %q", ytTask.ETA)
	}
	if ytTask.Downloaded != 1000 || ytTask.TotalBytes != 2000 {
		t.Errorf("expected yt-dlp bytes 1000/2000, got %d/%d", ytTask.Downloaded, ytTask.TotalBytes)
	}
}
