//go:build !unix

package cli

// noFollow: without O_NOFOLLOW (Windows, Plan 9…) only the EvalSymlinks
// done before the open resolves the path.
const noFollow = 0
