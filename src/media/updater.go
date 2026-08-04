package media

import (
	"bytes"
	"github.com/rs/zerolog/log"
	"io"
	"media-roller/src/utils"
	"os"
	"os/exec"
	"sync"
)

var CachedYtDlpVersion = ""

func UpdateYtDlp() error {
	log.Info().Msgf("Updateing yt-dlp to nightly")

	// Try yt-dlp's own updater first (works for standalone binary installs)
	err := runCommandOutput(exec.Command("yt-dlp", "-U"))
	if err == nil {
		log.Info().Msgf("Done updating yt-dlp. Version=%s", GetInstalledVersion())
		return nil
	}
	log.Warn().Msgf("yt-dlp self-update failed (%v), falling back to pip3", err)

	// Fall back to pip3, retrying with --break-system-packages (PEP 668)
	cmd := exec.Command("pip3", "install", "-U", "yt-dlp")
	if err := runCommandOutput(cmd); err == nil {
		log.Info().Msgf("Done updating yt-dlp. Version=%s", GetInstalledVersion())
		return nil
	}
	cmd = exec.Command("pip3", "install", "-U", "--break-system-packages", "yt-dlp")

	err = runCommandOutput(cmd)
	if err != nil {
		log.Error().Msgf("cmd.Run() failed with %s", err)
		return err
	}

	log.Info().Msgf("Done updating yt-dlp. Version=%s", GetInstalledVersion())

	return nil
}

// runCommandOutput runs a command, streaming its output to stdout/stderr, and returns any error.
func runCommandOutput(cmd *exec.Cmd) error {
	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutIn, _ := cmd.StdoutPipe()
	stderrIn, _ := cmd.StderrPipe()

	var errStdout, errStderr error
	stdout := io.MultiWriter(os.Stdout, &stdoutBuf)
	stderr := io.MultiWriter(os.Stderr, &stderrBuf)

	err := cmd.Start()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		_, errStdout = io.Copy(stdout, stdoutIn)
		wg.Done()
	}()

	_, errStderr = io.Copy(stderr, stderrIn)
	wg.Wait()

	err = cmd.Wait()
	if err != nil {
		return err
	} else if errStdout != nil {
		return errStdout
	} else if errStderr != nil {
		return errStderr
	}
	return nil
}

func GetInstalledVersion() string {
	version := utils.RunCommand("yt-dlp", "--version")
	if version == "" {
		version = "unknown"
	}
	CachedYtDlpVersion = version
	return version
}
