package libvirt

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

type DriverCancelCallback func(state multistep.StateBag) bool

// A driver is able to talk to libvirt-system-x86_64 and perform certain
// operations with it.
type Driver interface {
	// Copy bypasses libvirt-img convert and directly copies an image
	// that doesn't need converting.
	Copy(string, string) error

	// Stop stops a running machine, forcefully.
	Stop() error

	// Libvirt executes the given command via libvirt-system-x86_64
	Libvirt(libvirtArgs ...string) error

	// wait on shutdown of the VM with option to cancel
	WaitForShutdown(<-chan struct{}) bool

	// Libvirt executes the given command via libvirt-img
	LibvirtImg(...string) error

	// Verify checks to make sure that this driver should function
	// properly. If there is any indication the driver can't function,
	// this will return an error.
	Verify() error

	// Version reads the version of Libvirt that is installed.
	Version() (string, error)
}

type LibvirtDriver struct {
	LibvirtPath    string
	LibvirtImgPath string

	vmCmd   *exec.Cmd
	vmEndCh <-chan int
	lock    sync.Mutex
}

func (d *LibvirtDriver) Stop() error {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.vmCmd != nil {
		if err := d.vmCmd.Process.Kill(); err != nil {
			return err
		}
	}

	return nil
}

func (d *LibvirtDriver) Copy(sourceName, targetName string) error {
	source, err := os.Open(sourceName)
	if err != nil {
		err = fmt.Errorf("Error opening iso for copy: %s", err)
		return err
	}
	defer source.Close()

	// Create will truncate an existing file
	target, err := os.Create(targetName)
	if err != nil {
		err = fmt.Errorf("Error creating hard drive in output dir: %s", err)
		return err
	}
	defer target.Close()

	log.Printf("Copying %s to %s", source.Name(), target.Name())
	bytes, err := io.Copy(target, source)
	if err != nil {
		err = fmt.Errorf("Error copying iso to output dir: %s", err)
		return err
	}
	log.Printf(fmt.Sprintf("Copied %d bytes", bytes))

	return nil
}

func (d *LibvirtDriver) Libvirt(libvirtArgs ...string) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.vmCmd != nil {
		panic("Existing VM state found")
	}

	stdout_r, stdout_w := io.Pipe()
	stderr_r, stderr_w := io.Pipe()

	log.Printf("Executing %s: %#v", d.LibvirtPath, libvirtArgs)
	cmd := exec.Command(d.LibvirtPath, libvirtArgs...)
	cmd.Stdout = stdout_w
	cmd.Stderr = stderr_w

	err := cmd.Start()
	if err != nil {
		err = fmt.Errorf("Error starting VM: %s", err)
		return err
	}

	go logReader("Libvirt stdout", stdout_r)
	go logReader("Libvirt stderr", stderr_r)

	log.Printf("Started Libvirt. Pid: %d", cmd.Process.Pid)

	// Wait for Libvirt to complete in the background, and mark when its done
	endCh := make(chan int, 1)
	go func() {
		defer stderr_w.Close()
		defer stdout_w.Close()

		var exitCode int = 0
		if err := cmd.Wait(); err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				// The program has exited with an exit code != 0
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
				} else {
					exitCode = 254
				}
			}
		}

		endCh <- exitCode

		d.lock.Lock()
		defer d.lock.Unlock()
		d.vmCmd = nil
		d.vmEndCh = nil
	}()

	// Wait at least a couple seconds for an early fail from Libvirt so
	// we can report that.
	select {
	case exit := <-endCh:
		if exit != 0 {
			return fmt.Errorf("Libvirt failed to start. Please run with PACKER_LOG=1 to get more info.")
		}
	case <-time.After(2 * time.Second):
	}

	// Setup our state so we know we are running
	d.vmCmd = cmd
	d.vmEndCh = endCh

	return nil
}

func (d *LibvirtDriver) WaitForShutdown(cancelCh <-chan struct{}) bool {
	d.lock.Lock()
	endCh := d.vmEndCh
	d.lock.Unlock()

	if endCh == nil {
		return true
	}

	select {
	case <-endCh:
		return true
	case <-cancelCh:
		return false
	}
}

func (d *LibvirtDriver) LibvirtImg(args ...string) error {
	var stdout, stderr bytes.Buffer

	log.Printf("Executing libvirt-img: %#v", args)
	cmd := exec.Command(d.LibvirtImgPath, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	stdoutString := strings.TrimSpace(stdout.String())
	stderrString := strings.TrimSpace(stderr.String())

	if _, ok := err.(*exec.ExitError); ok {
		err = fmt.Errorf("LibvirtImg error: %s", stderrString)
	}

	log.Printf("stdout: %s", stdoutString)
	log.Printf("stderr: %s", stderrString)

	return err
}

func (d *LibvirtDriver) Verify() error {
	return nil
}

func (d *LibvirtDriver) Version() (string, error) {
	var stdout bytes.Buffer

	cmd := exec.Command(d.LibvirtPath, "-version")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	versionOutput := strings.TrimSpace(stdout.String())
	log.Printf("Libvirt --version output: %s", versionOutput)
	versionRe := regexp.MustCompile(`[\.[0-9]+]*`)
	matches := versionRe.FindStringSubmatch(versionOutput)
	if len(matches) == 0 {
		return "", fmt.Errorf("No version found: %s", versionOutput)
	}

	log.Printf("Libvirt version: %s", matches[0])
	return matches[0], nil
}

func logReader(name string, r io.Reader) {
	bufR := bufio.NewReader(r)
	for {
		line, err := bufR.ReadString('\n')
		if line != "" {
			line = strings.TrimRightFunc(line, unicode.IsSpace)
			log.Printf("%s: %s", name, line)
		}

		if err == io.EOF {
			break
		}
	}
}
