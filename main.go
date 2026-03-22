package main

import (
	"image/color"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Game struct{ mode int }

func (g *Game) Update() error {
	// 左键或空格：切换模式
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.mode++
		if g.mode >= 10 {
			os.Exit(0)
		}
	}
	// 右键或ESC：退出
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		os.Exit(0)
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	w, h := screen.Size()
	fw, fh := float32(w), float32(h)
	switch g.mode {
	case 0:
		screen.Fill(color.NRGBA{255, 0, 0, 255})
	case 1:
		screen.Fill(color.NRGBA{0, 255, 0, 255})
	case 2:
		screen.Fill(color.NRGBA{255, 255, 255, 255})
	case 3:
		screen.Fill(color.NRGBA{0, 0, 0, 255})
	case 4:
		screen.Fill(color.NRGBA{0, 0, 255, 255})
	case 5:
		screen.Fill(color.NRGBA{255, 255, 0, 255})
	case 6:
		screen.Fill(color.NRGBA{255, 0, 255, 255})
	case 7: // 渐变
		for i := 0; i < w; i++ {
			c := uint8(float32(i) / fw * 255)
			vector.StrokeLine(screen, float32(i), 0, float32(i), fh, 1, color.NRGBA{c, c, c, 255}, false)
		}
	case 8: // 网格
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
	ebiten.SetFullscreen(true)
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}
