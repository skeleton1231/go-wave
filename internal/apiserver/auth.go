package apiserver

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/skeleton1231/gotal/internal/apiserver/store"
	"github.com/skeleton1231/gotal/internal/apiserver/store/model"
	"github.com/skeleton1231/gotal/internal/pkg/middleware"
	"github.com/skeleton1231/gotal/internal/pkg/middleware/auth"
	"github.com/skeleton1231/gotal/pkg/log"
	"github.com/spf13/viper"
)

type loginInfo struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func newBasicAuth() middleware.AuthStrategy {
	return auth.NewBasicStrategy(func(username string, password string) bool {
		// fetch user from database
		user, err := store.Client().Users().GetByUsername(context.TODO(), username, model.GetOptions{})
		if err != nil {
			return false
		}

		// Compare the login password with the user password.
		if err := user.Compare(password); err != nil {
			return false
		}
		return true
	})
}

func newJWTAuth() middleware.AuthStrategy {
	ginjwt, _ := jwt.New(&jwt.GinJWTMiddleware{
		Realm:            viper.GetString("jwt.Realm"),
		SigningAlgorithm: "HS256",
		Key:              []byte(viper.GetString("jwt.key")),
		Timeout:          viper.GetDuration("jwt.timeout"),
		MaxRefresh:       viper.GetDuration("jwt.max-refresh"),
		Authenticator:    authenticator(),
		LoginResponse:    loginResponse(),
		LogoutResponse: func(c *gin.Context, code int) {
			c.JSON(http.StatusOK, nil)
		},
		RefreshResponse: refreshResponse(),
		PayloadFunc:     payloadFunc(),
		IdentityHandler: func(c *gin.Context) interface{} {
			claims := jwt.ExtractClaims(c)
			return claims[jwt.IdentityKey]
		},
		IdentityKey:  middleware.UsernameKey,
		Authorizator: authorizator(),
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.JSON(code, gin.H{
				"message": message,
			})
		},
		TokenLookup:   "header: Authorization, query: token, cookie: jwt",
		TokenHeadName: "Bearer",
		SendCookie:    true,
		TimeFunc:      time.Now,
	})

	return auth.NewJWTStrategy(*ginjwt)
}

func newAutoAuth() middleware.AuthStrategy {
	return auth.NewAutoStrategy(newBasicAuth().(auth.BasicStrategy), newJWTAuth().(auth.JWTStrategy))
}

func authenticator() func(c *gin.Context) (interface{}, error) {
	return func(c *gin.Context) (interface{}, error) {
		var login loginInfo
		var err error

		// support header and body both
		if c.Request.Header.Get("Authorization") != "" {
			login, err = parseWithHeader(c)
		} else {
			login, err = parseWithBody(c)
		}
		if err != nil {
			return "", jwt.ErrFailedAuthentication
		}

		log.Debugf("loginInfo username is %s, password is %s", login.Username, login.Password)

		// Get the user information by the login username.
		user, err := store.Client().Users().GetByUsername(c, login.Username, model.GetOptions{})
		if err != nil {
			log.Errorf("get user information failed: %s", err.Error())

			return "", jwt.ErrFailedAuthentication
		}

		// Compare the login password with the user password.
		if err := user.Compare(login.Password); err != nil {
			log.Errorf("password compare error is %s", err.Error())
			return "", jwt.ErrFailedAuthentication
		}

		return user, nil
	}
}

func parseWithHeader(c *gin.Context) (loginInfo, error) {
	auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
	if len(auth) != 2 || auth[0] != "Basic" {
		log.Errorf("get basic string from Authorization header failed")

		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	payload, err := base64.StdEncoding.DecodeString(auth[1])
	if err != nil {
		log.Errorf("decode basic string: %s", err.Error())

		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	pair := strings.SplitN(string(payload), ":", 2)

	if len(pair) != 2 {
		log.Errorf("parse payload failed")

		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	return loginInfo{
		Username: pair[0],
		Password: pair[1],
	}, nil
}

func parseWithBody(c *gin.Context) (loginInfo, error) {
	var login loginInfo
	if err := c.ShouldBindJSON(&login); err != nil {
		log.Errorf("parse login parameters: %s", err.Error())

		return loginInfo{}, jwt.ErrFailedAuthentication
	}

	return login, nil
}

func refreshResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

func loginResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

func payloadFunc() func(data interface{}) jwt.MapClaims {
	return func(data interface{}) jwt.MapClaims {
		claims := jwt.MapClaims{}
		if u, ok := data.(*model.User); ok {
			claims[jwt.IdentityKey] = u.Name // 用户名
			claims["userID"] = u.ID          // 用户ID
		}
		return claims
	}
}

// func authorizator() func(data interface{}, c *gin.Context) bool {
// 	return func(data interface{}, c *gin.Context) bool {
// 		if v, ok := data.(string); ok {
// 			log.Record(c).Infof("user `%s` is authenticated.", v)

// 			return true
// 		}

// 		return false
// 	}
// }

func authorizator() func(data interface{}, c *gin.Context) bool {
	return func(data interface{}, c *gin.Context) bool {
		if username, ok := data.(string); ok {
			log.Record(c).Infof("user `%s` is authenticated.", username)
			// 用户已经认证，现在检查他们是否有权限
			// 例如，这里可以构建一个请求到 authz 服务
			allowed, err := checkAuthzPermission(username, c.Request.Method, c.Request.URL.Path)
			if err != nil {
				// 处理可能的错误
				return false
			}

			return allowed
		}

		return false
	}
}

func checkAuthzPermission(username, method, path string) (bool, error) {
	// 构造请求到 authz 服务
	// 您需要根据实际的 authz API 接口来调整这里的代码
	// ...
	return true, nil
}
