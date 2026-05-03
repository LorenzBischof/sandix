package fuse

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/lorenzbischof/sandix/internal/wrapper"
)

const nixStorePath = "/nix/store"

// RootNode is the root of the sandboxed store FUSE filesystem.
// Only <hash>/bin/<name> paths are ever accessed (the rewriter guarantees this),
// so the tree is: RootNode → StoreEntryNode → BinDirNode → WrapperFileNode.
type RootNode struct {
	gofuse.Inode
	SandixPath string
}

var _ gofuse.NodeLookuper = (*RootNode)(nil)

func (r *RootNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	realPath := filepath.Join(nixStorePath, name)
	var st syscall.Stat_t
	if err := syscall.Lstat(realPath, &st); err != nil {
		return nil, gofuse.ToErrno(err)
	}
	out.FromStat(&st)
	child := r.NewPersistentInode(ctx, &StoreEntryNode{
		storeName:  name,
		sandixPath: r.SandixPath,
	}, gofuse.StableAttr{Mode: syscall.S_IFDIR})
	return child, 0
}

// StoreEntryNode represents a store path directory. Only bin/ is exposed.
type StoreEntryNode struct {
	gofuse.Inode
	storeName  string
	sandixPath string
}

var _ gofuse.NodeLookuper = (*StoreEntryNode)(nil)

func (n *StoreEntryNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	if name != "bin" {
		return nil, syscall.ENOENT
	}

	child := n.NewPersistentInode(ctx, &BinDirNode{
		storeName:  n.storeName,
		sandixPath: n.sandixPath,
	}, gofuse.StableAttr{Mode: syscall.S_IFDIR})
	return child, 0
}

// BinDirNode represents a bin/ directory inside a store path.
// Serves sandix exec wrapper scripts instead of real binaries.
type BinDirNode struct {
	gofuse.Inode
	storeName  string
	sandixPath string
}

var _ gofuse.NodeLookuper = (*BinDirNode)(nil)
var _ gofuse.NodeReaddirer = (*BinDirNode)(nil)

func (n *BinDirNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*gofuse.Inode, syscall.Errno) {
	realBin := filepath.Join(nixStorePath, n.storeName, "bin", name)
	if _, err := os.Lstat(realBin); err != nil {
		return nil, gofuse.ToErrno(err)
	}
	content := wrapper.Generate(n.storeName, name, n.sandixPath)
	child := n.NewPersistentInode(ctx, &WrapperFileNode{
		content: content,
	}, gofuse.StableAttr{Mode: syscall.S_IFREG})
	return child, 0
}

func (n *BinDirNode) Readdir(ctx context.Context) (gofuse.DirStream, syscall.Errno) {
	return gofuse.NewLoopbackDirStream(filepath.Join(nixStorePath, n.storeName, "bin"))
}

// WrapperFileNode represents a single sandix exec wrapper script.
type WrapperFileNode struct {
	gofuse.Inode
	content []byte
}

var _ gofuse.NodeGetattrer = (*WrapperFileNode)(nil)
var _ gofuse.NodeReader = (*WrapperFileNode)(nil)
var _ gofuse.NodeOpener = (*WrapperFileNode)(nil)

func (n *WrapperFileNode) Getattr(ctx context.Context, fh gofuse.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0555
	out.Size = uint64(len(n.content))
	return 0
}

func (n *WrapperFileNode) Open(ctx context.Context, flags uint32) (gofuse.FileHandle, uint32, syscall.Errno) {
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (n *WrapperFileNode) Read(ctx context.Context, fh gofuse.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if int(off) >= len(n.content) {
		return fuse.ReadResultData(nil), 0
	}
	end := int(off) + len(dest)
	if end > len(n.content) {
		end = len(n.content)
	}
	return fuse.ReadResultData(n.content[off:end]), 0
}
