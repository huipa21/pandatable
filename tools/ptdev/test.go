package main

import "context"

func (runner runner) test(ctx context.Context, opts options) error {
	var env map[string]string
	if !opts.noDB {
		if err := runner.db(ctx, "up", false); err != nil {
			return err
		}
		env = map[string]string{"PT_TEST_DATABASE_URL": dbDSN}
	}
	args := append([]string{"test", "./..."}, opts.testArgs...)
	if _, err := runner.execute(ctx, "", env, false, "go", args...); err != nil {
		return err
	}
	if runner.webExists() {
		return runner.pnpm(ctx, "test")
	}
	return nil
}
