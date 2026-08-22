//go:build windows

package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type _ICoreWebView2NavigationStartingEventArgsVtbl struct {
	_IUnknownVtbl
	GetUri             ComProc
	GetIsUserInitiated ComProc
	GetIsRedirected    ComProc
	GetRequestHeaders  ComProc
	GetCancel          ComProc
	PutCancel          ComProc
	GetNavigationId    ComProc
}

type ICoreWebView2NavigationStartingEventArgs struct {
	vtbl *_ICoreWebView2NavigationStartingEventArgsVtbl
}

func (i *ICoreWebView2NavigationStartingEventArgs) GetUri() (string, error) {
	var value *uint16
	hr, _, _ := i.vtbl.GetUri.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(&value)))
	if windows.Handle(hr) != windows.S_OK {
		return "", windows.Errno(hr)
	}
	result := windows.UTF16PtrToString(value)
	windows.CoTaskMemFree(unsafe.Pointer(value))
	return result, nil
}

func (i *ICoreWebView2NavigationStartingEventArgs) GetIsRedirected() (bool, error) {
	var value int32
	hr, _, _ := i.vtbl.GetIsRedirected.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(&value)))
	if windows.Handle(hr) != windows.S_OK {
		return false, windows.Errno(hr)
	}
	return value != 0, nil
}

func (i *ICoreWebView2NavigationStartingEventArgs) PutCancel(cancel bool) error {
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
