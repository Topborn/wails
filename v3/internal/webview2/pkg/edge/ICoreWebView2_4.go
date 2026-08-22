//go:build windows

package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type iCoreWebView2_4Vtbl struct {
	iCoreWebView2_3Vtbl
	AddFrameCreated        ComProc
	RemoveFrameCreated     ComProc
	AddDownloadStarting    ComProc
	RemoveDownloadStarting ComProc
}

type ICoreWebView2_4 struct {
	vtbl *iCoreWebView2_4Vtbl
}

func (i *ICoreWebView2_4) Release() uint32 {
	result, _, _ := i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
	return uint32(result)
}

func (i *ICoreWebView2_4) AddDownloadStarting(handler *ICoreWebView2DownloadStartingEventHandler, token *_EventRegistrationToken) error {
	hr, _, _ := i.vtbl.AddDownloadStarting.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(handler)),
		uintptr(unsafe.Pointer(token)),
	)
	if windows.Handle(hr) != windows.S_OK {
		return windows.Errno(hr)
	}
	return nil
}

func (i *ICoreWebView2) GetICoreWebView2_4() *ICoreWebView2_4 {
	var result *ICoreWebView2_4
	iid := NewGUID("{20D02D59-6DF2-42DC-BD06-F98A694B1302}")
	_, _, _ = i.vtbl.QueryInterface.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&result)),
	)
	return result
}
