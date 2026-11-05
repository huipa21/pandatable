package main

import "context"

func (runner runner) test(ctx context.Context, opts options) error {
	var env map[string]string
	if !opts.noDB {
		dsn, err := runner.database(ctx, "up", false)
		if err != nil {
			return err
		}
		env = map[string]string{"PT_TEST_DATABASE_URL": dsn, "PT_TEST_DATABASE_REQUIRED": "1"}
	}
	args := append([]string{"test", "./...", "-count=1"}, opts.testArgs...)
	if _, err := runner.execute(ctx, "", env, false, "go", args...); err != nil {
		return err
	}
	if runner.webExists() {
		return runner.pnpm(ctx, "test")
	}
	return nil
}
