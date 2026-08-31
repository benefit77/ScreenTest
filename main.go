//go:build !xp

package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"math/rand"
	"os"
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

	modeCount = 8
)

var keepAwakeTick time.Time

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
}

func (g *Game) Update() error {
	// 定期重新保持唤醒，防止系统在长时间运行后重置执行状态
	if time.Since(keepAwakeTick) > 2*time.Second {
		keepAwakeTick = time.Now()
		keepDisplayOn()
	}

	// F 键：切换闪烁修复模式
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		g.flashing = !g.flashing
	}

	mx, my := ebiten.CursorPosition()

	// ---- 鼠标拖动 ----
	leftJustPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	leftHeld := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	leftJustReleased := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)

	if leftJustPressed {
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
		if !g.isDragging {
			// 触摸按下：记录起点
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
			g.mode++
			if g.mode >= modeCount {
				os.Exit(0)
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
	case 0, 1, 2, 3, 4, 5, 6:
		clrs := []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {255, 255, 255, 255}, {0, 0, 0, 255}, {0, 0, 255, 255}, {255, 255, 0, 255}, {255, 0, 255, 255}}
		screen.Fill(clrs[g.mode])
	case 7: // 渐变
		for i := 0; i < w; i++ {
			c := uint8(float32(i) / fw * 255)
			vector.StrokeLine(screen, float32(i), 0, float32(i), fh, 1, color.NRGBA{c, c, c, 255}, false)
		}
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

func (g *Game) Layout(ow, oh int) (int, int) { return ow, oh }

func main() {
	ebiten.SetWindowDecorated(false)
	ebiten.SetFullscreen(true)
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	ebiten.SetWindowFloating(true)
	// 适当降低功耗，不需要 60FPS 也可以
	ebiten.SetVsyncEnabled(true)
	g := &Game{}
	keepAwakeTick = time.Now()
	keepDisplayOn() // 防息屏
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
