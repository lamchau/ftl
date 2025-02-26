package main

import (
	"context"
	"os"
	"path/filepath"

	ftlv1 "github.com/TBD54566975/ftl/backend/protos/xyz/block/ftl/v1"
	"github.com/TBD54566975/ftl/backend/protos/xyz/block/ftl/v1/ftlv1connect"
	"github.com/TBD54566975/ftl/internal/download"
	"github.com/TBD54566975/ftl/internal/model"
	"github.com/TBD54566975/ftl/internal/sha256"
)

type downloadCmd struct {
	Dest       string               `short:"d" help:"Destination directory." default:"."`
	Deployment model.DeploymentName `help:"Deployment to download." arg:""`
}

func (d *downloadCmd) Run(ctx context.Context, client ftlv1connect.ControllerServiceClient) error {
	return download.Artefacts(ctx, client, d.Deployment, d.Dest)
}

func (d *downloadCmd) getLocalArtefacts() ([]*ftlv1.DeploymentArtefact, error) {
	haveArtefacts := []*ftlv1.DeploymentArtefact{}
	dest, err := filepath.Abs(d.Dest)
	if err != nil {
		return nil, err
	}
	err = filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		sum, err := sha256.SumFile(path)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dest, path)
		if err != nil {
			return err
		}
		haveArtefacts = append(haveArtefacts, &ftlv1.DeploymentArtefact{
			Path:       relPath,
			Digest:     sum.String(),
			Executable: info.Mode()&0111 != 0,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return haveArtefacts, nil
}
