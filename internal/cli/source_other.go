//go:build !unix

package cli

// noFollow: Windows has no O_NOFOLLOW; EvalSymlinks already resolved the path.
const noFollow = 0
