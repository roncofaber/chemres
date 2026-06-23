package resolver

import "context"

type ctxKey int

const clientIPKey ctxKey = 0

func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}

func clientIPFromCtx(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}
