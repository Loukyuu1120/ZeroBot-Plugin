// Package wife 抽老婆
package wife

import (
	"bytes"
	"image"
	"image/color"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"

	zbmath "github.com/FloatTech/floatbox/math"
	"github.com/FloatTech/imgfactory"
)

var (
	sizeList = []int{0, 3, 5, 8}
	enguess  = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Help:             "- 猜老婆",
		Brief:            "从老婆库猜老婆",
	}).ApplySingle(ctxext.NewGroupSingle("已经有正在进行的游戏..."))
)

func init() {
	enguess.OnFullMatch("猜老婆").SetBlock(true).Limit(ctxext.LimitByUser).Handle(func(ctx *zero.Ctx) {
		class := 3

		if len(cards) == 0 {
		    ok := getJSON(ctx)
		    if !ok || len(cards) == 0 {
		        ctx.Send("老婆库还没准备好，请稍后再试~")
		        return
		    }
		}
		card := cards[rand.Intn(len(cards))]
		pic, err := engine.GetLazyData("wives/"+card, true)
		if err != nil {
			ctx.SendChain(message.Text("[猜老婆]error:\n", err))
			return
		}
		work, name := card2name(card)
		name = strings.ToLower(name)

		img, _, err := image.Decode(bytes.NewReader(pic))
		if err != nil {
			ctx.SendChain(message.Text("[猜老婆]解码图片失败:\n", err))
			return
		}
		dst := imgfactory.Size(img, img.Bounds().Dx(), img.Bounds().Dy())

		// ✅ 预生成 / 从缓存读取不同难度马赛克
		mosaicCache := make(map[int][]byte)
		for i := range sizeList {
			q, err := getCachedMosaic(card, dst, i)
			if err != nil {
				continue
			}
			mosaicCache[i] = q
		}

		// 发送初始图片
		if q, ok := mosaicCache[class]; ok {
			if id := ctx.SendChain(message.ImageBytes(q)); id.ID() != 0 {
				ctx.SendChain(message.Text("请回答该二次元角色名字\n以“xxx酱”格式回答\n发送“跳过”结束猜题"))
			}
		} else {
			ctx.SendChain(message.Text("[猜老婆]初始图片生成失败"))
			return
		}

		// 设置 FutureEvent
		var next *zero.FutureEvent
		if ctx.Event.GroupID == 0 {
			next = zero.NewFutureEvent("message", 999, false, zero.RegexRule(`^(·)?[^酱]+酱|^跳过$`), ctx.CheckSession())
		} else {
			next = zero.NewFutureEvent("message", 999, false, zero.RegexRule(`^(·)?[^酱]+酱|^跳过$`), zero.CheckGroup(ctx.Event.GroupID))
		}
		recv, cancel := next.Repeat()

		tick := time.After(105 * time.Second)
		after := time.After(120 * time.Second)

		for {
			select {
			case <-tick:
				ctx.SendChain(message.Text("[猜老婆]你还有15s作答时间"))

			case <-after:
				cancel()
				go ctx.Send(
					message.ReplyWithMessage(ctx.Event.MessageID,
						message.ImageBytes(pic),
						message.Text("[猜老婆]倒计时结束，游戏结束...\n角色是:\n", name, "\n出自《", work, "》\n"),
					),
				)
				return

			case c := <-recv:
				msg := strings.ReplaceAll(c.Event.Message.String(), "酱", "")
				msg = strings.TrimSpace(msg)
				if msg == "" {
					continue
				}

				if msg == "跳过" {
					cancel()
					if msgID := ctx.Send(message.ReplyWithMessage(c.Event.MessageID,
						message.Text("已跳过猜题\n角色是:\n", name, "\n出自《", work, "》\n"),
						message.ImageBytes(pic))); msgID.ID() == 0 {
						ctx.SendChain(message.Text("图片发送失败, 角色是:\n", name, "\n出自《", work, "》"))
					}
					return
				}

				class--
				if strings.Contains(name, strings.ToLower(msg)) {
					cancel()
					if msgID := ctx.Send(message.ReplyWithMessage(c.Event.MessageID,
						message.Text("太棒了,你猜对了!\n角色是:\n", name, "\n出自《", work, "》\n"),
						message.ImageBytes(pic))); msgID.ID() == 0 {
						ctx.SendChain(message.Text("太棒了,你猜对了!\n图片发送失败,角色是:\n", name, "\n出自《", work, "》"))
					}
					return
				}

				if class < 1 {
					cancel()
					if msgID := ctx.Send(message.ReplyWithMessage(c.Event.MessageID,
						message.Text("很遗憾,次数到了,游戏结束!\n角色是:\n", name, "\n出自《", work, "》\n"),
						message.ImageBytes(pic))); msgID.ID() == 0 {
						ctx.SendChain(message.Text("很遗憾,次数到了!\n图片发送失败,角色是:\n", name, "\n出自《", work, "》"))
					}
					return
				}

				// 答错，发送下一难度图片
				if q, ok := mosaicCache[class]; ok {
					hint := ""
					if class == 2 {
						hint = "(提示：" + work + ")\n"
					}
					ctx.SendChain(
						message.Text("回答错误,你还有", class, "次机会\n", hint, "请继续作答(难度降低)\n"),
						message.ImageBytes(q),
					)
				} else {
					ctx.SendChain(message.Text("回答错误，继续作答"))
				}
			}
		}
	})
}

// ✅ 带磁盘缓存的马赛克获取函数
func getCachedMosaic(card string, dst *imgfactory.Factory, level int) ([]byte, error) {
	cacheDir := engine.DataFolder() + "wives_modified"
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}
	cacheFile := filepath.Join(cacheDir, card+"_level"+strconv.Itoa(level)+".png")

	// 如果缓存存在，且能正常解码，直接用
	if data, err := ioutil.ReadFile(cacheFile); err == nil && len(data) > 0 {
		if _, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			return data, nil
		}
		// 文件损坏则删掉重建
		_ = os.Remove(cacheFile)
	}

	// 否则生成并保存
	q, err := mosaic(dst, level)
	if err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(cacheFile, q, 0644); err != nil {
		return nil, err
	}
	return q, nil
}

// 马赛克生成
func mosaic(dst *imgfactory.Factory, level int) ([]byte, error) {
	b := dst.Image().Bounds()
	p := imgfactory.NewFactoryBG(dst.W(), dst.H(), color.NRGBA{255, 255, 255, 255})
	markSize := zbmath.Max(b.Max.X, b.Max.Y) * sizeList[level] / 200
	if markSize < 1 {
		markSize = 1
	}

	for yOfMarknum := 0; yOfMarknum <= zbmath.Ceil(b.Max.Y, markSize); yOfMarknum++ {
		for xOfMarknum := 0; xOfMarknum <= zbmath.Ceil(b.Max.X, markSize); xOfMarknum++ {
			px := xOfMarknum*markSize + markSize/2
			py := yOfMarknum*markSize + markSize/2
			if px >= b.Max.X {
				px = b.Max.X - 1
			}
			if py >= b.Max.Y {
				py = b.Max.Y - 1
			}
			a := dst.Image().At(px, py)
			cc := color.NRGBAModel.Convert(a).(color.NRGBA)
			for y := 0; y < markSize; y++ {
				for x := 0; x < markSize; x++ {
					xOfPic := xOfMarknum*markSize + x
					yOfPic := yOfMarknum*markSize + y
					if xOfPic < b.Max.X && yOfPic < b.Max.Y {
						p.Image().Set(xOfPic, yOfPic, cc)
					}
				}
			}
		}
	}
	return imgfactory.ToBytes(p.Blur(3).Image())
}
