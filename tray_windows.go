//go:build windows

package main

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"elearning-sbt/handlers"
	"elearning-sbt/version"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procShowWindow            = user32.NewProc("ShowWindow")
	procUpdateWindow          = user32.NewProc("UpdateWindow")
	procInvalidateRect        = user32.NewProc("InvalidateRect")
	procBeginPaint            = user32.NewProc("BeginPaint")
	procEndPaint              = user32.NewProc("EndPaint")
	procGetClientRect         = user32.NewProc("GetClientRect")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procSetTimer              = user32.NewProc("SetTimer")
	procKillTimer             = user32.NewProc("KillTimer")
	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	procCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	procAppendMenuW           = user32.NewProc("AppendMenuW")
	procTrackPopupMenu        = user32.NewProc("TrackPopupMenu")
	procDestroyMenu           = user32.NewProc("DestroyMenu")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procCreateIconIndirect    = user32.NewProc("CreateIconIndirect")
	procDestroyIcon           = user32.NewProc("DestroyIcon")
	procFillRect              = user32.NewProc("FillRect")
	procDrawTextW             = user32.NewProc("DrawTextW")
	procLoadCursorW           = user32.NewProc("LoadCursorW")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procSetLayeredWindowAttribs  = user32.NewProc("SetLayeredWindowAttributes")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procCreatePen        = gdi32.NewProc("CreatePen")
	procCreateFontW      = gdi32.NewProc("CreateFontW")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procSetBkMode        = gdi32.NewProc("SetBkMode")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procRoundRect              = gdi32.NewProc("RoundRect")
	procCreateBitmap           = gdi32.NewProc("CreateBitmap")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procEllipse                = gdi32.NewProc("Ellipse")
	procMoveToEx               = gdi32.NewProc("MoveToEx")
	procLineTo                 = gdi32.NewProc("LineTo")
	procPolygon                = gdi32.NewProc("Polygon")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
)

const (
	wsPopup         = 0x80000000
	wsVisible       = 0x10000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	wsExLayered     = 0x00080000
	lwaAlpha        = 0x00000002
	swHide          = 0
	swShowNoAct     = 4
	swShow          = 5
	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmTimer         = 0x0113
	wmCommand       = 0x0111
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmMouseMove     = 0x0200
	wmTrayCallback  = 0x0400 + 42

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010
	niifInfo   = 0x00000001

	spiGetWorkArea = 0x0030
	smCxScreen     = 0
	smCyScreen     = 1

	tpmBottomAlign = 0x0020
	tpmLeftAlign   = 0x0000
	tpmRightButton = 0x0002
	mfString       = 0x0000
	mfGrayed       = 0x0001
	mfDisabled     = 0x0002
	mfSeparator    = 0x0800

	idcArrow     = 32512
	transparent  = 1
	dtLeft       = 0x0000
	dtCenter     = 0x0001
	dtWordBreak  = 0x0010
	dtSingleLine = 0x0020
	dtVCenter    = 0x0004

	swpNoActivate = 0x0010
	hwndTopmost   = ^uintptr(0) // -1
	srcCopy       = 0x00CC0020

	cmdOpenBrowser = 1001
	cmdTogglePopup = 1002
	cmdQuitApp     = 1003

	timerStartupHide = 1
	timerHoverCheck  = 2
	timerStartupAnim = 3
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type point struct {
	X int32
	Y int32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type msg struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type paintStruct struct {
	hdc         syscall.Handle
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             syscall.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            syscall.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     syscall.Handle
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  syscall.Handle
	hbmColor syscall.Handle
}

func rgb(r, g, b byte) uintptr {
	return uintptr(uint32(r) | (uint32(g) << 8) | (uint32(b) << 16))
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func copyUTF16(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(s)
	for i := 0; i < len(dst) && i < len(u); i++ {
		dst[i] = u[i]
	}
	if len(dst) > 0 {
		dst[len(dst)-1] = 0
	}
}

func createTrayIconHandle() syscall.Handle {
	const size = 32
	pixels := make([]byte, size*size*4) // BGRA
	mask := make([]byte, size*size/8)

	setPx := func(x, y int, r, g, b byte) {
		idx := (y*size + x) * 4
		pixels[idx+0] = b
		pixels[idx+1] = g
		pixels[idx+2] = r
		pixels[idx+3] = 255
	}

	inRoundedRect := func(x, y, x0, y0, x1, y1, rad int) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		cx, cy := 0, 0
		if x < x0+rad && y < y0+rad {
			cx, cy = x0+rad, y0+rad
		} else if x > x1-rad && y < y0+rad {
			cx, cy = x1-rad, y0+rad
		} else if x < x0+rad && y > y1-rad {
			cx, cy = x0+rad, y1-rad
		} else if x > x1-rad && y > y1-rad {
			cx, cy = x1-rad, y1-rad
		} else {
			return true
		}
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= rad*rad
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !inRoundedRect(x, y, 1, 1, 30, 30, 6) {
				byteIdx := (y*size + x) / 8
				bitIdx := uint(7 - ((y*size + x) % 8))
				mask[byteIdx] |= 1 << bitIdx
				continue
			}

			// Sky-blue to indigo badge gradient
			t := float64(x+y) / 60.0
			bgR := byte(14 + t*53)
			bgG := byte(165 - t*109)
			bgB := byte(233 - t*31)
			setPx(x, y, bgR, bgG, bgB)

			// Book (center)
			if x >= 8 && x <= 23 && y >= 5 && y <= 26 {
				if x >= 22 && y >= 6 && y <= 25 {
					setPx(x, y, 241, 245, 249) // White pages edge
				} else if x <= 10 {
					setPx(x, y, 59, 130, 246) // Book spine
				} else {
					setPx(x, y, 30, 58, 138) // Navy hardcover
				}
			}

			// Golden chain wrapped horizontally around the book
			inLeftLink := (x >= 5 && x <= 13 && y >= 13 && y <= 18) && !(x >= 7 && x <= 11 && y >= 15 && y <= 16)
			inRightLink := (x >= 18 && x <= 26 && y >= 13 && y <= 18) && !(x >= 20 && x <= 24 && y >= 15 && y <= 16)
			inCenterLink := (x >= 11 && x <= 20 && y >= 12 && y <= 19) && !(x >= 13 && x <= 18 && y >= 14 && y <= 17)
			if inLeftLink || inRightLink || inCenterLink {
				if y == 12 || y == 13 {
					setPx(x, y, 253, 224, 71) // Gold highlight
				} else {
					setPx(x, y, 245, 158, 11) // Rich amber gold
				}
			}
		}
	}

	hbmColor, _, _ := procCreateBitmap.Call(size, size, 1, 32, uintptr(unsafe.Pointer(&pixels[0])))
	hbmMask, _, _ := procCreateBitmap.Call(size, size, 1, 1, uintptr(unsafe.Pointer(&mask[0])))
	ii := iconInfo{
		fIcon:    1,
		hbmMask:  syscall.Handle(hbmMask),
		hbmColor: syscall.Handle(hbmColor),
	}
	hIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	procDeleteObject.Call(hbmColor)
	procDeleteObject.Call(hbmMask)
	return syscall.Handle(hIcon)
}

func drawText(hdc syscall.Handle, text string, r *rect, flags uintptr, color uintptr, fontSize int32, bold bool) {
	weight := int32(400)
	if bold {
		weight = 700
	}
	hFont, _, _ := procCreateFontW.Call(
		uintptr(fontSize), 0, 0, 0, uintptr(weight),
		0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(utf16Ptr("Segoe UI"))),
	)
	oldFont, _, _ := procSelectObject.Call(uintptr(hdc), hFont)
	procSetBkMode.Call(uintptr(hdc), transparent)
	procSetTextColor.Call(uintptr(hdc), color)
	u := syscall.StringToUTF16(text)
	procDrawTextW.Call(uintptr(hdc), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(r)), flags)
	procSelectObject.Call(uintptr(hdc), oldFont)
	procDeleteObject.Call(hFont)
}

func fillSolidRect(hdc syscall.Handle, r *rect, color uintptr) {
	brush, _, _ := procCreateSolidBrush.Call(color)
	procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(r)), brush)
	procDeleteObject.Call(brush)
}

func drawEllipseShape(hdc syscall.Handle, left, top, right, bottom int32, fillCol, borderCol uintptr) {
	brush, _, _ := procCreateSolidBrush.Call(fillCol)
	pen, _, _ := procCreatePen.Call(0, 1, borderCol)
	oldB, _, _ := procSelectObject.Call(uintptr(hdc), brush)
	oldP, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procEllipse.Call(uintptr(hdc), uintptr(left), uintptr(top), uintptr(right), uintptr(bottom))
	procSelectObject.Call(uintptr(hdc), oldB)
	procSelectObject.Call(uintptr(hdc), oldP)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

func drawRoundRectShape(hdc syscall.Handle, left, top, right, bottom, rx, ry int32, fillCol, borderCol uintptr) {
	brush, _, _ := procCreateSolidBrush.Call(fillCol)
	pen, _, _ := procCreatePen.Call(0, 1, borderCol)
	oldB, _, _ := procSelectObject.Call(uintptr(hdc), brush)
	oldP, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procRoundRect.Call(uintptr(hdc), uintptr(left), uintptr(top), uintptr(right), uintptr(bottom), uintptr(rx), uintptr(ry))
	procSelectObject.Call(uintptr(hdc), oldB)
	procSelectObject.Call(uintptr(hdc), oldP)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

func drawLineShape(hdc syscall.Handle, x1, y1, x2, y2, width int32, col uintptr) {
	pen, _, _ := procCreatePen.Call(0, uintptr(width), col)
	oldP, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procMoveToEx.Call(uintptr(hdc), uintptr(x1), uintptr(y1), 0)
	procLineTo.Call(uintptr(hdc), uintptr(x2), uintptr(y2))
	procSelectObject.Call(uintptr(hdc), oldP)
	procDeleteObject.Call(pen)
}

func drawPolyShape(hdc syscall.Handle, pts []point, fillCol, borderCol uintptr) {
	if len(pts) < 3 {
		return
	}
	brush, _, _ := procCreateSolidBrush.Call(fillCol)
	pen, _, _ := procCreatePen.Call(0, 1, borderCol)
	oldB, _, _ := procSelectObject.Call(uintptr(hdc), brush)
	oldP, _, _ := procSelectObject.Call(uintptr(hdc), pen)
	procPolygon.Call(uintptr(hdc), uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	procSelectObject.Call(uintptr(hdc), oldB)
	procSelectObject.Call(uintptr(hdc), oldP)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}

// drawCartoonNurseReading draws an animated pretty female cartoon nurse reading a book (smooth, flicker-free)
func drawCartoonNurseReading(hdc syscall.Handle, ox, oy int32, tick int) {
	// Gentle floating/reading bob (1-2 px smooth sine wave)
	bob := int32(math.Round(math.Sin(float64(tick)*0.12) * 1.8))
	pageShift := int32(math.Round(math.Sin(float64(tick)*0.18) * 3.0))
	blink := (tick % 56) >= 52 // natural brief eye blink

	// Soft pastel circular backdrop with subtle sky-pink glow
	drawEllipseShape(hdc, ox, oy, ox+88, oy+88, rgb(30, 41, 59), rgb(56, 189, 248))
	drawEllipseShape(hdc, ox+5, oy+5, ox+83, oy+83, rgb(38, 52, 74), rgb(38, 52, 74))

	// Long flowing chestnut hair behind shoulders
	hairMain := rgb(78, 49, 36)
	hairEdge := rgb(56, 33, 24)
	drawEllipseShape(hdc, ox+18, oy+24+bob, ox+70, oy+74+bob, hairMain, hairEdge)
	drawEllipseShape(hdc, ox+16, oy+38+bob, ox+32, oy+76+bob, hairMain, hairEdge)
	drawEllipseShape(hdc, ox+56, oy+38+bob, ox+72, oy+76+bob, hairMain, hairEdge)

	// Nurse body / tailored white uniform + soft sky-blue V-collar & pink heart pin
	drawRoundRectShape(hdc, ox+24, oy+54+bob, ox+64, oy+82, 14, 14, rgb(255, 255, 255), rgb(148, 163, 184))
	drawPolyShape(hdc, []point{
		{X: ox + 34, Y: oy + 54 + bob},
		{X: ox + 54, Y: oy + 54 + bob},
		{X: ox + 44, Y: oy + 67 + bob},
	}, rgb(56, 189, 248), rgb(14, 165, 233))
	// Neck
	drawRoundRectShape(hdc, ox+40, oy+49+bob, ox+48, oy+57+bob, 4, 4, rgb(255, 224, 198), rgb(240, 190, 155))

	// Pretty oval face (warm fair peach)
	drawEllipseShape(hdc, ox+28, oy+24+bob, ox+60, oy+54+bob, rgb(255, 228, 206), rgb(241, 194, 162))

	// Side hair strands & curved bangs framing face
	drawEllipseShape(hdc, ox+26, oy+21+bob, ox+46, oy+33+bob, hairMain, hairMain)
	drawEllipseShape(hdc, ox+42, oy+21+bob, ox+62, oy+33+bob, hairMain, hairMain)
	drawPolyShape(hdc, []point{
		{X: ox + 27, Y: oy + 29 + bob},
		{X: ox + 32, Y: oy + 35 + bob},
		{X: ox + 30, Y: oy + 46 + bob},
		{X: ox + 25, Y: oy + 42 + bob},
	}, hairMain, hairEdge)
	drawPolyShape(hdc, []point{
		{X: ox + 61, Y: oy + 29 + bob},
		{X: ox + 56, Y: oy + 35 + bob},
		{X: ox + 58, Y: oy + 46 + bob},
		{X: ox + 63, Y: oy + 42 + bob},
	}, hairMain, hairEdge)
	// Subtle hair shine
	drawLineShape(hdc, ox+33, oy+25+bob, ox+40, oy+24+bob, 2, rgb(146, 98, 72))
	drawLineShape(hdc, ox+49, oy+24+bob, ox+55, oy+26+bob, 2, rgb(146, 98, 72))

	// Gentle eyebrows
	drawLineShape(hdc, ox+33, oy+33+bob, ox+39, oy+32+bob, 1, rgb(78, 49, 36))
	drawLineShape(hdc, ox+49, oy+32+bob, ox+55, oy+33+bob, 1, rgb(78, 49, 36))

	// Expressive eyes with eyelashes & sparkle (or soft blink)
	if blink {
		drawLineShape(hdc, ox+33, oy+38+bob, ox+40, oy+38+bob, 2, rgb(45, 32, 32))
		drawLineShape(hdc, ox+48, oy+38+bob, ox+55, oy+38+bob, 2, rgb(45, 32, 32))
	} else {
		// Eye whites & dark iris
		drawEllipseShape(hdc, ox+33, oy+35+bob, ox+40, oy+42+bob, rgb(255, 255, 255), rgb(71, 85, 105))
		drawEllipseShape(hdc, ox+48, oy+35+bob, ox+55, oy+42+bob, rgb(255, 255, 255), rgb(71, 85, 105))
		drawEllipseShape(hdc, ox+34, oy+36+bob, ox+39, oy+41+bob, rgb(45, 32, 48), rgb(45, 32, 48))
		drawEllipseShape(hdc, ox+49, oy+36+bob, ox+54, oy+41+bob, rgb(45, 32, 48), rgb(45, 32, 48))
		// Eye sparkle highlights
		drawEllipseShape(hdc, ox+35, oy+36+bob, ox+37, oy+38+bob, rgb(255, 255, 255), rgb(255, 255, 255))
		drawEllipseShape(hdc, ox+50, oy+36+bob, ox+52, oy+38+bob, rgb(255, 255, 255), rgb(255, 255, 255))
		// Cute winged eyelashes
		drawLineShape(hdc, ox+32, oy+36+bob, ox+34, oy+35+bob, 1, rgb(30, 27, 38))
		drawLineShape(hdc, ox+54, oy+35+bob, ox+56, oy+36+bob, 1, rgb(30, 27, 38))
	}

	// Rosy pink cheeks
	drawEllipseShape(hdc, ox+30, oy+42+bob, ox+36, oy+46+bob, rgb(253, 164, 175), rgb(253, 164, 175))
	drawEllipseShape(hdc, ox+52, oy+42+bob, ox+58, oy+46+bob, rgb(253, 164, 175), rgb(253, 164, 175))

	// Sweet coral-pink smile
	drawLineShape(hdc, ox+41, oy+47+bob, ox+44, oy+49+bob, 1, rgb(244, 63, 94))
	drawLineShape(hdc, ox+44, oy+49+bob, ox+47, oy+47+bob, 1, rgb(244, 63, 94))

	// Classic Nurse Cap + Pink-Red Cross
	drawPolyShape(hdc, []point{
		{X: ox + 31, Y: oy + 22 + bob},
		{X: ox + 35, Y: oy + 11 + bob},
		{X: ox + 53, Y: oy + 11 + bob},
		{X: ox + 57, Y: oy + 22 + bob},
	}, rgb(255, 255, 255), rgb(203, 213, 225))
	rCrossV := rect{Left: ox + 43, Top: oy + 13 + bob, Right: ox + 45, Bottom: oy + 20 + bob}
	rCrossH := rect{Left: ox + 41, Top: oy + 15 + bob, Right: ox + 47, Bottom: oy + 18 + bob}
	fillSolidRect(hdc, &rCrossV, rgb(244, 63, 94))
	fillSolidRect(hdc, &rCrossH, rgb(244, 63, 94))

	// Open Book in hands (left page + right page + animated turning page)
	drawRoundRectShape(hdc, ox+18, oy+63, ox+44, oy+82, 4, 4, rgb(255, 255, 255), rgb(14, 165, 233))
	drawRoundRectShape(hdc, ox+44, oy+63, ox+70, oy+82, 4, 4, rgb(241, 245, 249), rgb(14, 165, 233))
	// Animated turning page
	drawRoundRectShape(hdc, ox+44+pageShift, oy+61, ox+58+pageShift, oy+80, 3, 3, rgb(224, 242, 254), rgb(56, 189, 248))
	// Cute little hands holding the book sides
	drawEllipseShape(hdc, ox+15, oy+69, ox+23, oy+76, rgb(255, 228, 206), rgb(241, 194, 162))
	drawEllipseShape(hdc, ox+65, oy+69, ox+73, oy+76, rgb(255, 228, 206), rgb(241, 194, 162))
}

// StartTrayUI shows the startup notification window, minimizes to the Windows System Tray,
// and displays a live hover popup with local IP, UserNode QR Code, and connected MasterNodes.
func StartTrayUI(port string, syncHandler *handlers.SyncHandler, onOpenBrowser func(), onQuit func()) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hInstRaw, _, _ := procGetModuleHandleW.Call(0)
		hInst := syscall.Handle(hInstRaw)
		hCursorRaw, _, _ := procLoadCursorW.Call(0, idcArrow)
		hCursor := syscall.Handle(hCursorRaw)
		hTrayIcon := createTrayIconHandle()

		var startupHwnd syscall.Handle
		var popupHwnd syscall.Handle
		var popupVisible bool
		var trayHoverAnchor point
		var hoverMissTicks int
		var animTick int
		const totalAnimTicks = 90 // ~2.7s at 30ms/tick

		showPopupAtCursor := func(fromTrayHover bool) {
			var pt point
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			if fromTrayHover {
				trayHoverAnchor = pt
			}

			var work rect
			procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&work)), 0)

			const popW = int32(340)
			const popH = int32(430)
			x := pt.X - popW/2
			y := pt.Y - popH - 16
			if x+popW > work.Right-8 {
				x = work.Right - popW - 8
			}
			if x < work.Left+8 {
				x = work.Left + 8
			}
			if y < work.Top+8 {
				y = work.Bottom - popH - 12
			}
			if y+popH > work.Bottom-4 {
				y = work.Bottom - popH - 8
			}

			procSetWindowPos.Call(
				uintptr(popupHwnd), hwndTopmost,
				uintptr(x), uintptr(y), uintptr(popW), uintptr(popH),
				swpNoActivate,
			)
			procInvalidateRect.Call(uintptr(popupHwnd), 0, 1)
			procShowWindow.Call(uintptr(popupHwnd), swShowNoAct)
			popupVisible = true
			hoverMissTicks = 0
			procSetTimer.Call(uintptr(popupHwnd), timerHoverCheck, 250, 0)
		}

		hidePopup := func() {
			if popupVisible {
				popupVisible = false
				procKillTimer.Call(uintptr(popupHwnd), timerHoverCheck)
				procShowWindow.Call(uintptr(popupHwnd), swHide)
			}
		}

		// Startup Notification Window Proc (Double-buffered, Zero Flicker, Cartoon Nurse Reading Book 0% -> 100%)
		startupProc := syscall.NewCallback(func(hwnd syscall.Handle, msgCode uint32, wParam, lParam uintptr) uintptr {
			switch msgCode {
			case wmPaint:
				var ps paintStruct
				hdcRaw, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
				screenHDC := syscall.Handle(hdcRaw)
				var cr rect
				procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&cr)))

				// Double-buffer offscreen DC so animation never flickers
				memDCRaw, _, _ := procCreateCompatibleDC.Call(uintptr(screenHDC))
				memDC := syscall.Handle(memDCRaw)
				memBmp, _, _ := procCreateCompatibleBitmap.Call(uintptr(screenHDC), uintptr(cr.Right), uintptr(cr.Bottom))
				oldBmp, _, _ := procSelectObject.Call(uintptr(memDC), memBmp)

				fillSolidRect(memDC, &cr, rgb(15, 23, 42))
				pen, _, _ := procCreatePen.Call(0, 2, rgb(56, 189, 248))
				brush, _, _ := procCreateSolidBrush.Call(rgb(15, 23, 42))
				oldPen, _, _ := procSelectObject.Call(uintptr(memDC), pen)
				oldBrush, _, _ := procSelectObject.Call(uintptr(memDC), brush)
				procRoundRect.Call(uintptr(memDC), 1, 1, uintptr(cr.Right-1), uintptr(cr.Bottom-1), 16, 16)
				procSelectObject.Call(uintptr(memDC), oldPen)
				procSelectObject.Call(uintptr(memDC), oldBrush)
				procDeleteObject.Call(pen)
				procDeleteObject.Call(brush)

				// Cartoon Nurse Reading a Book on the left
				drawCartoonNurseReading(memDC, 18, 30, animTick)

				// Smooth 0% -> 100% progress (reaches 100% at tick 72, holds briefly, then fades out)
				pctFloat := float64(animTick) / 72.0
				if pctFloat > 1.0 {
					pctFloat = 1.0
				}
				pctInt := int(pctFloat * 100.0)

				r1 := rect{Left: 120, Top: 24, Right: cr.Right - 18, Bottom: 52}
				drawText(memDC, "LearnChain เริ่มใช้งานแล้ว", &r1, dtLeft|dtSingleLine|dtVCenter, rgb(248, 250, 252), -18, true)

				status := syncHandler.GetTrayStatus(port)
				r2 := rect{Left: 120, Top: 54, Right: cr.Right - 18, Bottom: 76}
				drawText(memDC, status.UserNodeURL, &r2, dtLeft|dtSingleLine|dtVCenter, rgb(56, 189, 248), -13, true)

				// Progress bar
				barLeft := int32(120)
				barRight := cr.Right - 22
				barTop := int32(84)
				barBottom := int32(96)
				rTrack := rect{Left: barLeft, Top: barTop, Right: barRight, Bottom: barBottom}
				fillSolidRect(memDC, &rTrack, rgb(30, 41, 59))
				fillW := int32(float64(barRight-barLeft) * pctFloat)
				if fillW > 0 {
					rFill := rect{Left: barLeft, Top: barTop, Right: barLeft + fillW, Bottom: barBottom}
					fillSolidRect(memDC, &rFill, rgb(52, 211, 153))
				}

				r3 := rect{Left: 120, Top: 102, Right: cr.Right - 18, Bottom: 128}
				drawText(memDC, fmt.Sprintf("บทเรียนพร้อม %d%%", pctInt), &r3, dtLeft|dtSingleLine|dtVCenter, rgb(110, 231, 183), -13, true)

				procBitBlt.Call(uintptr(screenHDC), 0, 0, uintptr(cr.Right), uintptr(cr.Bottom), uintptr(memDC), 0, 0, srcCopy)
				procSelectObject.Call(uintptr(memDC), oldBmp)
				procDeleteObject.Call(memBmp)
				procDeleteDC.Call(uintptr(memDC))

				procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
				return 0

			case wmTimer:
				if wParam == timerStartupAnim {
					animTick++
					alpha := byte(255)
					if animTick <= 12 {
						alpha = byte((animTick * 255) / 12)
					} else if animTick >= totalAnimTicks-15 {
						rem := totalAnimTicks - animTick
						if rem < 0 {
							rem = 0
						}
						alpha = byte((rem * 255) / 15)
					}
					procSetLayeredWindowAttribs.Call(uintptr(hwnd), 0, uintptr(alpha), lwaAlpha)
					procInvalidateRect.Call(uintptr(hwnd), 0, 0)
					if animTick >= totalAnimTicks {
						procKillTimer.Call(uintptr(hwnd), timerStartupAnim)
						procShowWindow.Call(uintptr(hwnd), swHide)
					}
				}
				return 0

			case wmLButtonUp:
				procKillTimer.Call(uintptr(hwnd), timerStartupAnim)
				procShowWindow.Call(uintptr(hwnd), swHide)
				return 0

			case wmTrayCallback:
				ev := uint32(lParam & 0xFFFF)
				switch ev {
				case wmMouseMove:
					if !popupVisible {
						showPopupAtCursor(true)
					} else {
						var pt point
						procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
						trayHoverAnchor = pt
						hoverMissTicks = 0
					}
				case wmLButtonDblClk:
					hidePopup()
					if onOpenBrowser != nil {
						go onOpenBrowser()
					}
				case wmLButtonUp:
					if popupVisible {
						hidePopup()
					} else {
						showPopupAtCursor(true)
					}
				case wmRButtonUp:
					hidePopup()
					menuRaw, _, _ := procCreatePopupMenu.Call()
					hMenu := syscall.Handle(menuRaw)
					procAppendMenuW.Call(uintptr(hMenu), mfString|mfGrayed|mfDisabled, 0, uintptr(unsafe.Pointer(utf16Ptr("LearnChain "+version.Version))))
					procAppendMenuW.Call(uintptr(hMenu), mfSeparator, 0, 0)
					procAppendMenuW.Call(uintptr(hMenu), mfString, cmdOpenBrowser, uintptr(unsafe.Pointer(utf16Ptr("🌐 เปิด LearnChain"))))
					procAppendMenuW.Call(uintptr(hMenu), mfString, cmdTogglePopup, uintptr(unsafe.Pointer(utf16Ptr("📡 แสดง QR & Node"))))
					procAppendMenuW.Call(uintptr(hMenu), mfSeparator, 0, 0)
					procAppendMenuW.Call(uintptr(hMenu), mfString, cmdQuitApp, uintptr(unsafe.Pointer(utf16Ptr("❌ ปิดโปรแกรม"))))

					var pt point
					procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
					procSetForegroundWindow.Call(uintptr(hwnd))
					procTrackPopupMenu.Call(uintptr(hMenu), tpmBottomAlign|tpmLeftAlign|tpmRightButton, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(hwnd), 0)
					procDestroyMenu.Call(uintptr(hMenu))
				}
				return 0

			case wmCommand:
				cmd := uint32(wParam & 0xFFFF)
				switch cmd {
				case cmdOpenBrowser:
					if onOpenBrowser != nil {
						go onOpenBrowser()
					}
				case cmdTogglePopup:
					if popupVisible {
						hidePopup()
					} else {
						showPopupAtCursor(false)
					}
				case cmdQuitApp:
					var nid notifyIconDataW
					nid.cbSize = uint32(unsafe.Sizeof(nid))
					nid.hWnd = hwnd
					nid.uID = 1
					procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
					if hTrayIcon != 0 {
						procDestroyIcon.Call(uintptr(hTrayIcon))
					}
					if onQuit != nil {
						onQuit()
					}
					procPostQuitMessage.Call(0)
				}
				return 0

			case wmDestroy:
				procPostQuitMessage.Call(0)
				return 0
			}
			r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msgCode), wParam, lParam)
			return r
		})

		// Hover Popup Window Proc (IP + UserNode QR Code + MasterNode list)
		popupProc := syscall.NewCallback(func(hwnd syscall.Handle, msgCode uint32, wParam, lParam uintptr) uintptr {
			switch msgCode {
			case wmPaint:
				var ps paintStruct
				hdcRaw, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
				hdc := syscall.Handle(hdcRaw)
				var cr rect
				procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&cr)))

				// Background + Border
				fillSolidRect(hdc, &cr, rgb(15, 23, 42))
				pen, _, _ := procCreatePen.Call(0, 2, rgb(56, 189, 248))
				brush, _, _ := procCreateSolidBrush.Call(rgb(15, 23, 42))
				oldPen, _, _ := procSelectObject.Call(uintptr(hdc), pen)
				oldBrush, _, _ := procSelectObject.Call(uintptr(hdc), brush)
				procRoundRect.Call(uintptr(hdc), 1, 1, uintptr(cr.Right-1), uintptr(cr.Bottom-1), 14, 14)
				procSelectObject.Call(uintptr(hdc), oldPen)
				procSelectObject.Call(uintptr(hdc), oldBrush)
				procDeleteObject.Call(pen)
				procDeleteObject.Call(brush)

				status := syncHandler.GetTrayStatus(port)

				// Title
				rTitle := rect{Left: 14, Top: 12, Right: cr.Right - 14, Bottom: 36}
				drawText(hdc, "🩺 LearnChain", &rTitle, dtCenter|dtSingleLine|dtVCenter, rgb(56, 189, 248), -16, true)

				// UserNode IP
				rIP := rect{Left: 14, Top: 38, Right: cr.Right - 14, Bottom: 62}
				ipText := fmt.Sprintf("UserNode: %s", status.UserNodeURL)
				drawText(hdc, ipText, &rIP, dtCenter|dtSingleLine|dtVCenter, rgb(248, 250, 252), -13, true)

				// QR Code Box (White quiet zone)
				const qrBoxSize = int32(180)
				qrLeft := (cr.Right - qrBoxSize) / 2
				qrTop := int32(68)
				rQRBox := rect{Left: qrLeft, Top: qrTop, Right: qrLeft + qrBoxSize, Bottom: qrTop + qrBoxSize}
				fillSolidRect(hdc, &rQRBox, rgb(255, 255, 255))

				modules, n := handlers.BuildQRMatrix(status.UserNodeURL)
				if n > 0 {
					cell := int32(5)
					for (n+6)*int(cell) > int(qrBoxSize) && cell > 1 {
						cell--
					}
					totalQRPx := int32(n) * cell
					startX := qrLeft + (qrBoxSize-totalQRPx)/2
					startY := qrTop + (qrBoxSize-totalQRPx)/2
					darkBrush, _, _ := procCreateSolidBrush.Call(rgb(15, 23, 42))
					for r := 0; r < n; r++ {
						for c := 0; c < n; c++ {
							if modules[r][c] {
								mr := rect{
									Left:   startX + int32(c)*cell,
									Top:    startY + int32(r)*cell,
									Right:  startX + int32(c+1)*cell,
									Bottom: startY + int32(r+1)*cell,
								}
								procFillRect.Call(uintptr(hdc), uintptr(unsafe.Pointer(&mr)), darkBrush)
							}
						}
					}
					procDeleteObject.Call(darkBrush)
				}

				// MasterNode Section (concise)
				rMasterHdr := rect{Left: 16, Top: 258, Right: cr.Right - 16, Bottom: 280}
				drawText(hdc, fmt.Sprintf("MasterNode (%d)", len(status.MasterNodes)), &rMasterHdr, dtLeft|dtSingleLine|dtVCenter, rgb(250, 204, 21), -13, true)

				var lines []string
				for i, n := range status.MasterNodes {
					if i >= 5 {
						lines = append(lines, fmt.Sprintf("+ อีก %d เครื่อง", len(status.MasterNodes)-5))
						break
					}
					host := n.Hostname
					if host == "" {
						host = n.NodeID
					}
					tag := "Peer"
					if n.IsSelf {
						tag = "Master"
					}
					lines = append(lines, fmt.Sprintf("• [%s] %s (%s)", tag, host, n.LanURL))
				}
				if len(lines) == 0 {
					lines = append(lines, fmt.Sprintf("• [Master] %s", status.UserNodeURL))
				}
				rMasterList := rect{Left: 16, Top: 284, Right: cr.Right - 16, Bottom: 416}
				drawText(hdc, strings.Join(lines, "\n"), &rMasterList, dtLeft|dtWordBreak, rgb(203, 213, 225), -12, false)

				procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
				return 0

			case wmTimer:
				if wParam == timerHoverCheck && popupVisible {
					var pt point
					procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
					var wr rect
					procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wr)))

					inPopup := pt.X >= wr.Left-8 && pt.X <= wr.Right+8 && pt.Y >= wr.Top-8 && pt.Y <= wr.Bottom+8
					dx := pt.X - trayHoverAnchor.X
					dy := pt.Y - trayHoverAnchor.Y
					if dx < 0 {
						dx = -dx
					}
					if dy < 0 {
						dy = -dy
					}
					nearTray := dx <= 38 && dy <= 38

					if inPopup || nearTray {
						hoverMissTicks = 0
					} else {
						hoverMissTicks++
						if hoverMissTicks >= 2 {
							hidePopup()
						}
					}
				}
				return 0
			}
			r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msgCode), wParam, lParam)
			return r
		})

		startupClass := utf16Ptr("LearnChainStartupClass")
		wcStartup := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   startupProc,
			hInstance:     hInst,
			hIcon:         hTrayIcon,
			hCursor:       hCursor,
			lpszClassName: startupClass,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcStartup)))

		popupClass := utf16Ptr("LearnChainPopupClass")
		wcPopup := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   popupProc,
			hInstance:     hInst,
			hCursor:       hCursor,
			lpszClassName: popupClass,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcPopup)))

		scrW, _, _ := procGetSystemMetrics.Call(smCxScreen)
		scrH, _, _ := procGetSystemMetrics.Call(smCyScreen)
		const sw = int32(380)
		const sh = int32(150)
		sx := (int32(scrW) - sw) / 2
		sy := (int32(scrH) - sh) / 2

		shWndRaw, _, _ := procCreateWindowExW.Call(
			wsExTopmost|wsExToolWindow|wsExLayered,
			uintptr(unsafe.Pointer(startupClass)),
			uintptr(unsafe.Pointer(utf16Ptr("LearnChain"))),
			wsPopup|wsVisible,
			uintptr(sx), uintptr(sy), uintptr(sw), uintptr(sh),
			0, 0, uintptr(hInst), 0,
		)
		startupHwnd = syscall.Handle(shWndRaw)
		procSetLayeredWindowAttribs.Call(uintptr(startupHwnd), 0, 0, lwaAlpha)
		procShowWindow.Call(uintptr(startupHwnd), swShow)
		procUpdateWindow.Call(uintptr(startupHwnd))
		procSetTimer.Call(uintptr(startupHwnd), timerStartupAnim, 30, 0)

		phWndRaw, _, _ := procCreateWindowExW.Call(
			wsExTopmost|wsExToolWindow|wsExNoActivate,
			uintptr(unsafe.Pointer(popupClass)),
			uintptr(unsafe.Pointer(utf16Ptr("LearnChain Node Status"))),
			wsPopup,
			0, 0, 340, 430,
			0, 0, uintptr(hInst), 0,
		)
		popupHwnd = syscall.Handle(phWndRaw)

		// Add System Tray Icon + Balloon Notification
		status := syncHandler.GetTrayStatus(port)
		var nid notifyIconDataW
		nid.cbSize = uint32(unsafe.Sizeof(nid))
		nid.hWnd = startupHwnd
		nid.uID = 1
		nid.uFlags = nifMessage | nifIcon | nifTip | nifInfo
		nid.uCallbackMessage = wmTrayCallback
		nid.hIcon = hTrayIcon
		nid.dwInfoFlags = niifInfo
		copyUTF16(nid.szTip[:], fmt.Sprintf("LearnChain • %s", status.UserNodeURL))
		copyUTF16(nid.szInfoTitle[:], "LearnChain พร้อมใช้งาน")
		copyUTF16(nid.szInfo[:], status.UserNodeURL)
		procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))

		var m msg
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(ret) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
}
