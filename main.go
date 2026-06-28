//go:build !xp

package main

import (
	"image/color"
	"log"
	"math/rand"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Game struct {
	mode     int
	flashing bool
}

func (g *Game) Update() error {
	// F 键：切换闪烁修复模式
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		g.flashing = !g.flashing
	}

	if !g.flashing {
		// 左键/空格/右方向键：下一步
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyRight) {
			g.mode++
			if g.mode >= 10 {
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
	case 8: // 网格
		screen.Fill(color.Black)
		for i := 0; i <= 10; i++ {
			vector.StrokeLine(screen, 0, float32(i)*(fh/10), fw, float32(i)*(fh/10), 1, color.NRGBA{100, 100, 100, 255}, false)
			vector.StrokeLine(screen, float32(i)*(fw/10), 0, float32(i)*(fw/10), fh, 1, color.NRGBA{100, 100, 100, 255}, false)
		}
	case 9: // 对比度
		for i := 0; i <= 10; i++ {
			val := uint8(float32(i) / 100.0 * 255.0)
			vector.DrawFilledRect(screen, float32(i)*(fw/11), 0, fw/11, fh, color.NRGBA{val, val, val, 255}, false)
		}
	}
}

func (g *Game) Layout(ow, oh int) (int, int) { return ow, oh }

func main() {
	ebiten.SetWindowDecorated(false)
	ebiten.SetFullscreen(true)
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	ebiten.SetWindowFloating(true)
	// 适当降低功耗，不需要 60FPS
	ebiten.SetVsyncEnabled(true)
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}
