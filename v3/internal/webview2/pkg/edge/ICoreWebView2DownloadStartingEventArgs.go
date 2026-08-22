//go:build windows

package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type _ICoreWebView2DownloadStartingEventArgsVtbl struct {
	_IUnknownVtbl
	GetDownloadOperation ComProc
	GetCancel            ComProc
	PutCancel            ComProc
	GetResultFilePath    ComProc
	PutResultFilePath    ComProc
	GetHandled           ComProc
	PutHandled           ComProc
	GetDeferral          ComProc
}

type ICoreWebView2DownloadStartingEventArgs struct {
	vtbl *_ICoreWebView2DownloadStartingEventArgsVtbl
}

func (i *ICoreWebView2DownloadStartingEventArgs) PutCancel(cancel bool) error {
	var value int32
	if cancel {
		value = 1
	}
	hr, _, _ := i.vtbl.PutCancel.Call(uintptr(unsafe.Pointer(i)), uintptr(value))
	if windows.Handle(hr) != windows.S_OK {
		return windows.Errno(hr)
	}
	return nil
}

func (i *ICoreWebView2DownloadStartingEventArgs) PutHandled(handled bool) error {
	var value int32
	if handled {
		value = 1
	}
	hr, _, _ := i.vtbl.PutHandled.Call(uintptr(unsafe.Pointer(i)), uintptr(value))
	if windows.Handle(hr) != windows.S_OK {
		return windows.Errno(hr)
	}
	return nil
}
