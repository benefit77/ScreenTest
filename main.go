//go:build !xp

package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	clickThreshold = 10.0 // 拖动超过此像素则算拖动，不触发点击

	holdExitTime   = 2 * time.Second // 长按此时间退出
	holdCancelDist = 15.0            // 长按期间手指移动超过此像素则取消

	modeTouchTest = 1  // 断触测试（第二个模式，彩条之后）
	modeCalib     = 12 // 触摸校准（最后一个模式）
	modeCount     = 13

	calibMargin = 50.0 // 校准十字准星离屏幕边缘的距离（像素）
)

type Game struct {
	mode     int
	flashing bool

	// 拖动框（普通模式）
	dragStartX, dragStartY float32 // 按下起点
	dragEndX, dragEndY     float32 // 当前终点
	isDragging             bool    // 是否正在拖动
	prevTouchIDs           []ebiten.TouchID

	// 长按退出
	holdActive   bool
	holdStart    time.Time
	holdX, holdY float32

	// 触摸校准
	prevMode             int
	calibValid           bool
	calScaleX, calOffX   float64
	calScaleY, calOffY   float64
	calibStep            int // 0..3 记录中，4 完成
	calibExpX, calibExpY [4]float64
	calibRawX, calibRawY [4]float64
	calibJustFinished    bool // 刚完成校准的那一次松手不切页面
}

func (g *Game) Update() error {
	// F 键：切换闪烁修复模式
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		g.flashing = !g.flashing
	}

	mx, my := ebiten.CursorPosition()
	// 进入校准模式时重置校准步骤；校准模式下使用原始坐标，其他模式应用校准
	if g.mode == modeCalib && g.prevMode != modeCalib {
		g.calibStep = 0
	}
	g.prevMode = g.mode
	if g.mode != modeCalib {
		fx, fy := g.applyCalib(float64(mx), float64(my))
		mx, my = int(fx), int(fy)
	}

	// ---- 鼠标拖动 ----
	leftJustPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	leftHeld := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	leftJustReleased := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)

	if leftJustPressed {
		if g.mode == modeCalib && g.calibStep < 4 {
			g.recordCalibPoint(float64(mx), float64(my))
		}
		g.dragStartX, g.dragStartY = float32(mx), float32(my)
		g.dragEndX, g.dragEndY = float32(mx), float32(my)
		g.isDragging = true
		g.holdActive = true
		g.holdStart = time.Now()
		g.holdX, g.holdY = float32(mx), float32(my)
	}

	if leftHeld && g.isDragging {
		g.dragEndX, g.dragEndY = float32(mx), float32(my)
	}

	// ---- 触摸拖动 ----
	curTouchIDs := ebiten.AppendTouchIDs(nil)
	if len(curTouchIDs) > 0 {
		id := curTouchIDs[0]
		tx, ty := ebiten.TouchPosition(id)
		if g.mode != modeCalib {
			fx, fy := g.applyCalib(float64(tx), float64(ty))
			tx, ty = int(fx), int(fy)
		}
		if !g.isDragging {
			// 触摸按下：记录起点
			if g.mode == modeCalib && g.calibStep < 4 {
				g.recordCalibPoint(float64(tx), float64(ty))
			}
			g.dragStartX, g.dragStartY = float32(tx), float32(ty)
			g.dragEndX, g.dragEndY = float32(tx), float32(ty)
			g.isDragging = true
			g.holdActive = true
			g.holdStart = time.Now()
			g.holdX, g.holdY = float32(tx), float32(ty)
		} else {
			// 触摸移动：更新终点
			g.dragEndX, g.dragEndY = float32(tx), float32(ty)
		}
	}

	// ---- 长按退出检测 ----
	if g.holdActive {
		dx := g.dragEndX - g.holdX
		dy := g.dragEndY - g.holdY
		if math.Hypot(float64(dx), float64(dy)) > holdCancelDist {
			// 手指移动了：取消长按（当作拖动）
			g.holdActive = false
		} else if time.Since(g.holdStart) >= holdExitTime {
			os.Exit(0) // 长按 2 秒退出
		}
	}

	// 检测触摸抬起
	touchJustReleased := false
	for _, prevID := range g.prevTouchIDs {
		found := false
		for _, curID := range curTouchIDs {
			if prevID == curID {
				found = true
				break
			}
		}
		if !found {
			touchJustReleased = true
			break
		}
	}
	g.prevTouchIDs = curTouchIDs
	if touchJustReleased {
		g.isDragging = false
	}

	// ---- 处理点击/拖动松开 ----
	justReleased := leftJustReleased || touchJustReleased
	if justReleased {
		g.isDragging = false
		g.holdActive = false
		// 计算按下到松开移动了多少像素
		dx := g.dragEndX - g.dragStartX
		dy := g.dragEndY - g.dragStartY
		dist := math.Sqrt(float64(dx*dx + dy*dy))

		if dist < clickThreshold && !g.flashing {
			// 这是点击（非拖动）
			// 校准模式下点已用于记录校准点，不切换模式
			if g.mode != modeCalib || g.calibStep >= 4 {
				if g.mode == modeCalib && g.calibStep >= 4 {
					if g.calibJustFinished {
						// 完成校准的那一次松手：只显示完成提示，不切页面
						g.calibJustFinished = false
					} else {
						// 校准完成：点击回到断触画布，方便立刻验证
						g.mode = modeTouchTest
					}
				} else {
					g.mode++
					if g.mode >= modeCount {
						os.Exit(0)
					}
				}
			}
		}
		// 如果 dist >= clickThreshold，是拖动，什么都不做
	}

	if !g.flashing {
		// 空格/右方向键：下一步
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyRight) {
			g.mode++
			if g.mode >= modeCount {
				os.Exit(0)
			}
		}
		// 左方向键：上一步
		if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
			g.mode--
			if g.mode < 0 {
				g.mode = 0
			}
		}
	}

	// 右键/ESC：退出
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		os.Exit(0)
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	w, h := screen.Size()
	fw, fh := float32(w), float32(h)

	if g.flashing {
		// 随机闪烁模式
		screen.Fill(color.NRGBA{uint8(rand.Intn(255)), uint8(rand.Intn(255)), uint8(rand.Intn(255)), 255})
		return
	}

	switch g.mode {
	case 0: // 彩条
		bars := []color.NRGBA{{255, 255, 255, 255}, {255, 255, 0, 255}, {0, 255, 255, 255}, {0, 255, 0, 255}, {255, 0, 255, 255}, {255, 0, 0, 255}, {0, 0, 255, 255}}
		for i, c := range bars {
			vector.DrawFilledRect(screen, float32(i)*(fw/float32(len(bars))), 0, fw/float32(len(bars)), fh, c, false)
		}
	case modeTouchTest: // 断触测试画布
		g.drawTouchTest(screen, w, h)
	case 2, 3, 4, 5, 6, 7, 8:
		clrs := []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {255, 255, 255, 255}, {0, 0, 0, 255}, {0, 0, 255, 255}, {255, 255, 0, 255}, {255, 0, 255, 255}}
		screen.Fill(clrs[g.mode-2])
	case 9: // 渐变
		for i := 0; i < w; i++ {
			c := uint8(float32(i) / fw * 255)
			vector.StrokeLine(screen, float32(i), 0, float32(i), fh, 1, color.NRGBA{c, c, c, 255}, false)
		}
	case 10: // 网格
		screen.Fill(color.Black)
		for i := 0; i <= 10; i++ {
			vector.StrokeLine(screen, 0, float32(i)*(fh/10), fw, float32(i)*(fh/10), 1, color.NRGBA{100, 100, 100, 255}, false)
			vector.StrokeLine(screen, float32(i)*(fw/10), 0, float32(i)*(fw/10), fh, 1, color.NRGBA{100, 100, 100, 255}, false)
		}
	case 11: // 对比度
		for i := 0; i <= 10; i++ {
			val := uint8(float32(i) / 100.0 * 255.0)
			vector.DrawFilledRect(screen, float32(i)*(fw/11), 0, fw/11, fh, color.NRGBA{val, val, val, 255}, false)
		}
	case modeCalib: // 触摸校准
		g.drawCalib(screen, w, h)
	}

	// ---- 绘制拖动框（所有模式：手指拖动时显示从按下点到当前位置的框） ----
	if g.isDragging {
		x1, y1 := g.dragStartX, g.dragStartY
		x2, y2 := g.dragEndX, g.dragEndY
		// 确保 x1<x2, y1<y2
		if x1 > x2 {
			x1, x2 = x2, x1
		}
		if y1 > y2 {
			y1, y2 = y2, y1
		}
		// 半透明填充
		vector.DrawFilledRect(screen, x1, y1, x2-x1, y2-y1, color.NRGBA{100, 150, 255, 40}, false)
		// 实线边框
		vector.StrokeRect(screen, x1, y1, x2-x1, y2-y1, 2, color.NRGBA{100, 150, 255, 220}, false)
		// 虚线效果用点线模拟：画 4 个角标
		cornerLen := float32(12)
		// 左上角
		vector.StrokeLine(screen, x1, y1, x1+cornerLen, y1, 2, color.White, false)
		vector.StrokeLine(screen, x1, y1, x1, y1+cornerLen, 2, color.White, false)
		// 右上角
		vector.StrokeLine(screen, x2, y1, x2-cornerLen, y1, 2, color.White, false)
		vector.StrokeLine(screen, x2, y1, x2, y1+cornerLen, 2, color.White, false)
		// 左下角
		vector.StrokeLine(screen, x1, y2, x1+cornerLen, y2, 2, color.White, false)
		vector.StrokeLine(screen, x1, y2, x1, y2-cornerLen, 2, color.White, false)
		// 右下角
		vector.StrokeLine(screen, x2, y2, x2-cornerLen, y2, 2, color.White, false)
		vector.StrokeLine(screen, x2, y2, x2, y2-cornerLen, 2, color.White, false)

		// 尺寸文字
		bw := int(x2 - x1)
		bh := int(y2 - y1)
		label := fmt.Sprintf("%d x %d", bw, bh)
		ebitenutil.DebugPrintAt(screen, label, int(x1)+4, int(y1)-16)
	}

	// ---- 长按退出进度圈 ----
	if g.holdActive {
		prog := float64(time.Since(g.holdStart)) / float64(holdExitTime)
		if prog > 1 {
			prog = 1
		}
		// 背景圈
		vector.StrokeCircle(screen, g.holdX, g.holdY, 36, 2, color.NRGBA{255, 255, 255, 90}, false)
		// 进度分段（24 段）
		const segs = 24
		lit := int(prog * segs)
		for i := 0; i < lit; i++ {
			a0 := -math.Pi/2 + float64(i)*2*math.Pi/segs
			a1 := -math.Pi/2 + float64(i+1)*2*math.Pi/segs
			x0 := g.holdX + 36*float32(math.Cos(a0))
			y0 := g.holdY + 36*float32(math.Sin(a0))
			x1 := g.holdX + 36*float32(math.Cos(a1))
			y1 := g.holdY + 36*float32(math.Sin(a1))
			vector.StrokeLine(screen, x0, y0, x1, y1, 3, color.NRGBA{0, 220, 255, 255}, false)
		}
		ebitenutil.DebugPrintAt(screen, "release to cancel", int(g.holdX)-44, int(g.holdY)+48)
	}
}

// drawTouchTest 绘制断触测试画布。
func (g *Game) drawTouchTest(screen *ebiten.Image, w, h int) {
	// 深色背景
	screen.Fill(color.NRGBA{16, 20, 26, 255})
	// 浅色网格
	for x := 40; x < w; x += 40 {
		vector.StrokeLine(screen, float32(x), 0, float32(x), float32(h), 1, color.NRGBA{38, 46, 58, 255}, false)
	}
	for y := 40; y < h; y += 40 {
		vector.StrokeLine(screen, 0, float32(y), float32(w), float32(y), 1, color.NRGBA{38, 46, 58, 255}, false)
	}
	// 提示文字
	info := "Drag on the canvas to draw a box\nSpace/Right: next mode   ESC: exit"
	ebitenutil.DebugPrintAt(screen, info, 12, 12)
}

// ---- 触摸校准 ----

func (g *Game) calibTargets(w, h int) {
	g.calibExpX[0], g.calibExpY[0] = calibMargin, float64(h)/2
	g.calibExpX[1], g.calibExpY[1] = float64(w)-calibMargin, float64(h)/2
	g.calibExpX[2], g.calibExpY[2] = float64(w)/2, calibMargin
	g.calibExpX[3], g.calibExpY[3] = float64(w)/2, float64(h)-calibMargin
}

func (g *Game) recordCalibPoint(x, y float64) {
	g.calibRawX[g.calibStep], g.calibRawY[g.calibStep] = x, y
	g.calibStep++
	if g.calibStep >= 4 {
		g.finishCalib()
	}
}

func (g *Game) finishCalib() {
	dxExp := g.calibExpX[1] - g.calibExpX[0]
	dxRep := g.calibRawX[1] - g.calibRawX[0]
	dyExp := g.calibExpY[3] - g.calibExpY[2]
	dyRep := g.calibRawY[3] - g.calibRawY[2]
	if math.Abs(dxRep) < 20 || math.Abs(dyRep) < 20 {
		// 无效（两次点得太近），重新校准
		g.calibStep = 0
		return
	}
	g.calScaleX = dxExp / dxRep
	g.calOffX = g.calibExpX[0] - g.calibRawX[0]*g.calScaleX
	g.calScaleY = dyExp / dyRep
	g.calOffY = g.calibExpY[2] - g.calibRawY[2]*g.calScaleY
	g.calibValid = true
	g.calibStep = 4
	g.calibJustFinished = true
	g.saveCalib()
}

func (g *Game) applyCalib(x, y float64) (float64, float64) {
	if !g.calibValid {
		return x, y
	}
	return x*g.calScaleX + g.calOffX, y*g.calScaleY + g.calOffY
}

func calibPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "screentest_calib.txt")
}

func (g *Game) saveCalib() {
	p := calibPath()
	if p == "" {
		return
	}
	data := fmt.Sprintf("%.6f %.6f %.6f %.6f\n", g.calScaleX, g.calOffX, g.calScaleY, g.calOffY)
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		return
	}
}

func (g *Game) loadCalib() {
	p := calibPath()
	if p == "" {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var sx, ox, sy, oy float64
	if _, err := fmt.Sscanf(string(data), "%f %f %f %f", &sx, &ox, &sy, &oy); err != nil {
		return
	}
	// 合理性检查
	if sx < 0.5 || sx > 2 || sy < 0.5 || sy > 2 {
		return
	}
	g.calScaleX, g.calOffX, g.calScaleY, g.calOffY = sx, ox, sy, oy
	g.calibValid = true
}

func (g *Game) drawCalib(screen *ebiten.Image, w, h int) {
	// 深色背景 + 网格
	screen.Fill(color.NRGBA{16, 20, 26, 255})
	for x := 40; x < w; x += 40 {
		vector.StrokeLine(screen, float32(x), 0, float32(x), float32(h), 1, color.NRGBA{38, 46, 58, 255}, false)
	}
	for y := 40; y < h; y += 40 {
		vector.StrokeLine(screen, 0, float32(y), float32(w), float32(y), 1, color.NRGBA{38, 46, 58, 255}, false)
	}
	g.calibTargets(w, h)
	if g.calibStep < 4 {
		tx, ty := g.calibExpX[g.calibStep], g.calibExpY[g.calibStep]
		// 十字准星
		vector.StrokeLine(screen, float32(tx-30), float32(ty), float32(tx+30), float32(ty), 2, color.White, false)
		vector.StrokeLine(screen, float32(tx), float32(ty-30), float32(tx), float32(ty+30), 2, color.White, false)
		vector.StrokeCircle(screen, float32(tx), float32(ty), 14, 2, color.NRGBA{0, 220, 255, 255}, false)
		labels := []string{
			"Calibration: touch the LEFT crosshair",
			"Calibration: touch the RIGHT crosshair",
			"Calibration: touch the TOP crosshair",
			"Calibration: touch the BOTTOM crosshair",
		}
		ebitenutil.DebugPrintAt(screen, labels[g.calibStep], 12, 12)
		ebitenutil.DebugPrintAt(screen, "Touch and release at the crosshair", 12, 30)
	} else {
		ebitenutil.DebugPrintAt(screen, "Calibration saved.", 12, 12)
		ebitenutil.DebugPrintAt(screen, "Tap to go to the touch canvas.", 12, 30)
	}
}

func (g *Game) Layout(ow, oh int) (int, int) { return ow, oh }

func main() {
	ebiten.SetWindowDecorated(false)
	ebiten.SetFullscreen(true)
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	ebiten.SetWindowFloating(true)
	// 适当降低功耗，不需要 60FPS 也可以
	ebiten.SetVsyncEnabled(true)
	g := &Game{}
	g.loadCalib() // 加载触摸校准
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
