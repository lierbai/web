package web

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/lierbai/web/internal/bytesconv"
)

// AuthUserKey 是基本身份验证中用户凭据的cookie名称.
const AuthUserKey = "user"

// Accounts 为授权登录的user/pass列表定义key/value
type Accounts map[string]string

type authPair struct {
	value string
	user  string
}

type authPairs []authPair

func (a authPairs) searchCredential(authValue string) (string, bool) {
	if authValue == "" {
		return "", false
	}
	for _, pair := range a {
		if pair.value == authValue {
			return pair.user, true
		}
	}
	return "", false
}

// BasicAuthForRealm 返回一个基本的HTTP授权中间件.map[string]string作为参数.
// 其中 密钥是用户名,值是密码,以及域的名称.
// 如果域为空，则默认使用"需要授权"
// (see http://tools.ietf.org/html/rfc2617#section-1.2)
func BasicAuthForRealm(accounts Accounts, realm string) HandlerFunc {
	if realm == "" {
		realm = "Authorization Required"
	}
	realm = "Basic realm=" + strconv.Quote(realm)
	pairs := processAccounts(accounts)
	return func(c *Context) {
		// 在允许的凭据片段中搜索用户
		user, found := pairs.searchCredential(c.requestHeader("Authorization"))
		if !found {
			// 凭证不匹配,返回401并中止处理程序链
			c.Header("WWW-Authenticate", realm)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 找到用户凭据, set user's id to key AuthUserKey in this context, the user's id can be read later using
		// c.MustGet(gin.AuthUserKey).
		c.Set(AuthUserKey, user)
	}
}

// BasicAuth 返回一个基本的HTTP授权中间件. 它接收 map[string]string作为参数
// the key is the user name and the value is the password.
func BasicAuth(accounts Accounts) HandlerFunc {
	return BasicAuthForRealm(accounts, "")
}

func processAccounts(accounts Accounts) authPairs {
	assert1(len(accounts) > 0, "Empty list of authorized credentials")
	pairs := make(authPairs, 0, len(accounts))
	for user, password := range accounts {
		assert1(user != "", "User can not be empty")
		value := authorizationHeader(user, password)
		pairs = append(pairs, authPair{
			value: value,
			user:  user,
		})
	}
	return pairs
}

func authorizationHeader(user, password string) string {
	base := user + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString(bytesconv.StringToBytes(base))
}
