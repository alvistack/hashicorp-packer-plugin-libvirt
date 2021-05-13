package libvirt

import "sync"

type DriverMock struct {
	sync.Mutex

	CopyCalled bool
	CopyErr    error

	StopCalled bool
	StopErr    error

	LibvirtCalls [][]string
	LibvirtErrs  []error

	WaitForShutdownCalled bool
	WaitForShutdownState  bool

	LibvirtImgCalled bool
	LibvirtImgCalls  []string
	LibvirtImgErrs   []error

	VerifyCalled bool
	VerifyErr    error

	VersionCalled bool
	VersionResult string
	VersionErr    error
}

func (d *DriverMock) Copy(source, dst string) error {
	d.CopyCalled = true
	return d.CopyErr
}

func (d *DriverMock) Stop() error {
	d.StopCalled = true
	return d.StopErr
}

func (d *DriverMock) Libvirt(args ...string) error {
	d.LibvirtCalls = append(d.LibvirtCalls, args)

	if len(d.LibvirtErrs) >= len(d.LibvirtCalls) {
		return d.LibvirtErrs[len(d.LibvirtCalls)-1]
	}
	return nil
}

func (d *DriverMock) WaitForShutdown(cancelCh <-chan struct{}) bool {
	d.WaitForShutdownCalled = true
	return d.WaitForShutdownState
}

func (d *DriverMock) LibvirtImg(args ...string) error {
	d.LibvirtImgCalled = true
	d.LibvirtImgCalls = append(d.LibvirtImgCalls, args...)

	if len(d.LibvirtImgErrs) >= len(d.LibvirtImgCalls) {
		return d.LibvirtImgErrs[len(d.LibvirtImgCalls)-1]
	}
	return nil
}

func (d *DriverMock) Verify() error {
	d.VerifyCalled = true
	return d.VerifyErr
}

func (d *DriverMock) Version() (string, error) {
	d.VersionCalled = true
	return d.VersionResult, d.VersionErr
}
