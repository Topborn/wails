//go:build windows

package edge

import "unsafe"

type _ICoreWebView2DownloadStartingEventHandlerVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}

type ICoreWebView2DownloadStartingEventHandler struct {
	vtbl *_ICoreWebView2DownloadStartingEventHandlerVtbl
	impl _ICoreWebView2DownloadStartingEventHandlerImpl
}

type _ICoreWebView2DownloadStartingEventHandlerImpl interface {
	_IUnknownImpl
	DownloadStarting(sender *ICoreWebView2, args *ICoreWebView2DownloadStartingEventArgs) uintptr
}

func _ICoreWebView2DownloadStartingEventHandlerIUnknownQueryInterface(this *ICoreWebView2DownloadStartingEventHandler, refiid, object uintptr) uintptr {
	return this.impl.QueryInterface(refiid, object)
}

func _ICoreWebView2DownloadStartingEventHandlerIUnknownAddRef(this *ICoreWebView2DownloadStartingEventHandler) uintptr {
	return this.impl.AddRef()
}

func _ICoreWebView2DownloadStartingEventHandlerIUnknownRelease(this *ICoreWebView2DownloadStartingEventHandler) uintptr {
	return this.impl.Release()
}

func _ICoreWebView2DownloadStartingEventHandlerInvoke(this *ICoreWebView2DownloadStartingEventHandler, sender *ICoreWebView2, args *ICoreWebView2DownloadStartingEventArgs) uintptr {
	return this.impl.DownloadStarting(sender, args)
}

var _ICoreWebView2DownloadStartingEventHandlerFn = _ICoreWebView2DownloadStartingEventHandlerVtbl{
	_IUnknownVtbl: _IUnknownVtbl{
		QueryInterface: NewComProc(_ICoreWebView2DownloadStartingEventHandlerIUnknownQueryInterface),
		AddRef:         NewComProc(_ICoreWebView2DownloadStartingEventHandlerIUnknownAddRef),
		Release:        NewComProc(_ICoreWebView2DownloadStartingEventHandlerIUnknownRelease),
	},
	Invoke: NewComProc(_ICoreWebView2DownloadStartingEventHandlerInvoke),
}

func newICoreWebView2DownloadStartingEventHandler(impl _ICoreWebView2DownloadStartingEventHandlerImpl) *ICoreWebView2DownloadStartingEventHandler {
	return &ICoreWebView2DownloadStartingEventHandler{
		vtbl: &_ICoreWebView2DownloadStartingEventHandlerFn,
		impl: impl,
	}
}

func (i *ICoreWebView2DownloadStartingEventHandler) AddRef() uintptr {
	result, _, _ := i.vtbl.AddRef.Call(uintptr(unsafe.Pointer(i)))
	return result
}
