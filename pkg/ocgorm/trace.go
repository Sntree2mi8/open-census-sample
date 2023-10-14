package ocgorm

import (
	"context"
	"github.com/jinzhu/gorm"
	"go.opencensus.io/trace"
)

const (
	contextKey string = "ocgorm_context_key"
	spanKey    string = "ocgorm_span_key"
)

func WithContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if ctx == nil {
		return db
	}
	return db.New().Set(contextKey, ctx)
}

func RegisterCallbacks(db *gorm.DB) {
	db.Callback().Query().
		Before("gorm:query").
		Register(
			"ocgorm:before_query",
			func(scope *gorm.Scope) {
				setItem, _ := scope.Get(contextKey)
				setCtx, ok := setItem.(context.Context)
				if !ok || setCtx == nil {
					return
				}

				if pSpan := trace.FromContext(setCtx); pSpan == nil {
					return
				}
				_, span := trace.StartSpan(setCtx, "gorm:query")

				scope.Set(spanKey, span)
				scope.Set(contextKey, setCtx)
			},
		)
	db.Callback().Query().
		After("gorm:query").
		Register(
			"ocgorm:after_query",
			func(scope *gorm.Scope) {
				item, ok := scope.Get(spanKey)
				if !ok || item == nil {
					return
				}

				span, ok := item.(*trace.Span)
				if !ok {
					return
				}

				var status trace.Status
				if scope.HasError() {
					err := scope.DB().Error
					if gorm.IsRecordNotFoundError(err) {
						status.Code = trace.StatusCodeNotFound
					} else {
						status.Code = trace.StatusCodeUnknown
					}

					status.Message = err.Error()
				}

				span.AddAttributes(trace.StringAttribute("gorm.query", scope.SQL))
				span.SetStatus(status)
				span.End()
			},
		)
}
