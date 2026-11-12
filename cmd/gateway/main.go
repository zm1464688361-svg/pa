package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/kitex/client"
	petshop "github.com/zm1464688361-svg/pa/kitex_gen/petshop/appointmentservice"
	"github.com/zm1464688361-svg/pa/internal/config"
	"github.com/zm1464688361-svg/pa/internal/model"
	"github.com/zm1464688361-svg/pa/internal/repo"
	"github.com/zm1464688361-svg/pa/internal/session"
	"gorm.io/gorm"
)

//go:embed templates/*.html
var templateFS embed.FS

var (
	db        *gorm.DB
	svc       petshop.Client
	templates *template.Template
)

func main() {
	dsn := config.LoadDSN()
	var err error
	db, err = repo.Open(dsn, true)
	if err != nil {
		panic(err)
	}
	svc, err = petshop.NewClient("appointmentservice",
		client.WithHostPorts(getenv("RPC_HOSTPORT", "127.0.0.1:9001")))
	if err != nil {
		panic(err)
	}
	templates = template.Must(template.New("").Funcs(template.FuncMap{
		"fen": func(v int64) string { return fmt.Sprintf("¥%d.%02d", v/100, v%100) },
		"shortTime": func(s string) string {
			if i := strings.IndexByte(s, ' '); i >= 0 {
				return s[i+1:]
			}
			return s
		},
	}).ParseFS(templateFS, "templates/*.html"))

	h := server.Default(server.WithHostPorts(getenv("HTTP_ADDR", ":8081")))

	// 登录/登出
	h.GET("/login", hLogin)
	h.POST("/login", hLoginPost)
	h.GET("/logout", hLogout)
	// 首页按角色跳转
	h.GET("/", hHome)

	// 宠物主人
	h.GET("/slots", hSlots)
	h.POST("/bookings", hCreateBooking)
	h.GET("/bookings", hMyBookings)
	h.GET("/bookings/:id", hBookingDetail)
	h.POST("/bookings/:id/cancel-unpaid", hCancelUnpaid)
	h.GET("/bookings/:id/cancel", hCancelForm)
	h.POST("/bookings/:id/cancel", hSubmitCancel)
	// 模拟支付触发（经 pay_sim 异步回调）
	h.POST("/pay/:orderNo", hTriggerPay)
	h.GET("/pay-hold/:orderNo", hPayHold)

	// pay_sim 回调入口（异步）
	h.POST("/callback/pay", hPayCallback)
	h.POST("/callback/refund", hRefundCallback)

	// 前台
	h.GET("/front", hFront)
	h.POST("/front/:id/checkin", hCheckIn)
	h.POST("/front/:id/start", hStart)
	h.POST("/front/:id/complete", hComplete)

	// 客服
	h.GET("/support", hSupport)
	h.POST("/support/review", hReviewCancel)
	h.GET("/support/refunds", hSupportRefunds)
	h.POST("/support/refunds/:id/retry", hRetryRefund)
	h.GET("/support/abnormal", hSupportAbnormal)
	h.POST("/support/abnormal/:id/refund", hRefundAbnormal)

	// 店长
	h.GET("/owner", hOwner)
	h.POST("/owner/slots", hCreateSlot)
	h.POST("/owner/noshow", hMarkNoShow)
	h.POST("/owner/noshow/:id/reason", hNoShowReason)

	// 教学时钟
	h.GET("/clock", hClock)
	h.POST("/clock", hClockSet)

	h.Spin()
}

// ---------- 会话 ----------

func currentUser(c *app.RequestContext) *model.User {
	tok := string(c.Cookie(session.CookieName()))
	if tok == "" {
		return nil
	}
	uid, err := session.Parse(tok)
	if err != nil {
		return nil
	}
	var u model.User
	if err := db.First(&u, uid).Error; err != nil {
		return nil
	}
	return &u
}

func requireUser(c *app.RequestContext) *model.User {
	u := currentUser(c)
	if u == nil {
		c.Redirect(302, []byte("/login"))
	}
	return u
}

// actor 会话用户转 RPC Actor。
func actor(u *model.User) *actorT {
	return &actorT{UserId: u.ID, Role: u.Role, ShopId: u.ShopID}
}

// ---------- 渲染 ----------

func render(c *app.RequestContext, page string, data map[string]interface{}) {
	if data == nil {
		data = map[string]interface{}{}
	}
	u := currentUser(c)
	data["User"] = u
	data["ClockNow"] = time.Now().Format("2006-01-02 15:04")
	if u != nil {
		data["RoleName"] = map[string]string{
			"pet_owner": "宠物主人", "front": "前台/服务人员", "support": "客服", "owner": "店长",
		}[u.Role]
	}
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, page, data); err != nil {
		c.String(500, "template error: "+err.Error())
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Write(buf.Bytes())
}

func homeOf(u *model.User) string {
	switch u.Role {
	case "pet_owner":
		return "/slots"
	case "front":
		return "/front"
	case "support":
		return "/support"
	case "owner":
		return "/owner"
	}
	return "/login"
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func atoi64(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }

var _ = http.StatusOK
