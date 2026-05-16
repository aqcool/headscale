package util

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
	"tailscale.com/types/logger"
)

func LogErr(err error, msg string) {
	log.Printf("[ERROR] %s: %v", msg, err)
}

func TSLogfWrapper() logger.Logf {
	return func(format string, args ...any) {
		log.Printf("[DEBUG] %s", fmt.Sprintf(format, args...))
	}
}

type DBLogWrapper struct {
	SlowThreshold         time.Duration
	SkipErrRecordNotFound bool
	ParameterizedQueries  bool
}

func NewDBLogWrapper(slowThreshold time.Duration, skipErrRecordNotFound bool, parameterizedQueries bool) *DBLogWrapper {
	return &DBLogWrapper{
		SlowThreshold:         slowThreshold,
		SkipErrRecordNotFound: skipErrRecordNotFound,
		ParameterizedQueries:  parameterizedQueries,
	}
}

func (l *DBLogWrapper) LogMode(gormLogger.LogLevel) gormLogger.Interface {
	return l
}

func (l *DBLogWrapper) Info(ctx context.Context, msg string, data ...any) {
	log.Printf("[INFO] %s", fmt.Sprintf(msg, data...))
}

func (l *DBLogWrapper) Warn(ctx context.Context, msg string, data ...any) {
	log.Printf("[WARN] %s", fmt.Sprintf(msg, data...))
}

func (l *DBLogWrapper) Error(ctx context.Context, msg string, data ...any) {
	log.Printf("[ERROR] %s", fmt.Sprintf(msg, data...))
}

func (l *DBLogWrapper) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rowsAffected := fc()

	if err != nil && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.SkipErrRecordNotFound) {
		log.Printf("[ERROR] sql error: %v, duration=%v, sql=%s, rows=%d", err, elapsed, sql, rowsAffected)
		return
	}

	if l.SlowThreshold != 0 && elapsed > l.SlowThreshold {
		log.Printf("[WARN] slow query: duration=%v, sql=%s, rows=%d", elapsed, sql, rowsAffected)
		return
	}

	log.Printf("[DEBUG] sql: duration=%v, sql=%s, rows=%d", elapsed, sql, rowsAffected)
}

func (l *DBLogWrapper) ParamsFilter(ctx context.Context, sql string, params ...any) (string, []any) {
	if l.ParameterizedQueries {
		return sql, nil
	}

	return sql, params
}