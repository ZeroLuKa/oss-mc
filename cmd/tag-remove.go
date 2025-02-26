// Copyright (c) 2015-2022 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"context"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/minio/cli"
	json "github.com/minio/colorjson"
	"github.com/trinet2005/oss-mc/pkg/probe"
	"github.com/trinet2005/oss-pkg/console"
)

var tagRemoveFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "version-id, vid",
		Usage: "remove tags on a specific object version",
	},
	cli.StringFlag{
		Name:  "rewind",
		Usage: "remove tags on an object version at specified time",
	},
	cli.BoolFlag{
		Name:  "versions",
		Usage: "remove tags on multiple versions of an object",
	},
	cli.BoolFlag{
		Name:  "recursive, r",
		Usage: "recursivley remove tags for all objects",
	},
}

var tagRemoveCmd = cli.Command{
	Name:         "remove",
	Usage:        "remove tags assigned to a bucket or an object",
	Action:       mainRemoveTag,
	OnUsageError: onUsageError,
	Before:       setGlobalsFromContext,
	Flags:        append(tagRemoveFlags, globalFlags...),
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} [COMMAND FLAGS] TARGET

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
DESCRIPTION:
  Remove tags assigned to a bucket or an object.

EXAMPLES:
  1. Remove the tags assigned to an object.
     {{.Prompt}} {{.HelpName}} myminio/testbucket/testobject

  2. Remove the tags assigned to a particular version of an object.
     {{.Prompt}} {{.HelpName}} --version-id "ieQq7aXsyhlhDt47YURGlrucYY3GxWHa" myminio/testbucket/testobject

  3. Remove the tags assigned to an object versions that are older than one week
     {{.Prompt}} {{.HelpName}} --versions --rewind 7d myminio/testbucket/testobject

  4. Remove the tags assigned to a bucket.
     {{.Prompt}} {{.HelpName}} play/testbucket

  5. Remove the tags recursively for all the objects of subdirs of bucket.
     {{.Prompt}} {{.HelpName}} --recursive myminio/testbucket

  6. Remove the tags recursively for all versions of all objects of subdirs of bucket.
     {{.Prompt}} {{.HelpName}} --recursive --versions myminio/testbucket
`,
}

// tagSetTagMessage structure will show message depending on the type of console.
type tagRemoveMessage struct {
	Status    string `json:"status"`
	Name      string `json:"name"`
	VersionID string `json:"versionID"`
}

// tagRemoveMessage console colorized output.
func (t tagRemoveMessage) String() string {
	var msg string
	msg += "Tags removed for " + t.Name
	if strings.TrimSpace(t.VersionID) != "" {
		msg += " (" + t.VersionID + ")"
	}
	msg += "."
	return console.Colorize("Remove", msg)
}

// JSON tagRemoveMessage.
func (t tagRemoveMessage) JSON() string {
	msgBytes, e := json.MarshalIndent(t, "", " ")
	fatalIf(probe.NewError(e), "Unable to marshal into JSON.")
	return string(msgBytes)
}

func parseRemoveTagSyntax(ctx *cli.Context) (targetURL, versionID string, timeRef time.Time, withVersions, recursive bool) {
	if len(ctx.Args()) != 1 {
		showCommandHelpAndExit(ctx, globalErrorExitStatus)
	}

	targetURL = ctx.Args().Get(0)
	versionID = ctx.String("version-id")
	withVersions = ctx.Bool("versions")
	rewind := ctx.String("rewind")
	recursive = ctx.Bool("recursive")

	if versionID != "" && (rewind != "" || withVersions) {
		fatalIf(errDummy().Trace(), "You cannot specify both --version-id and --rewind or --versions flags at the same time")
	}

	timeRef = parseRewindFlag(rewind)
	return
}

// Delete tags of a bucket or a specified object/version
func deleteTags(ctx context.Context, clnt Client, versionID string) {
	targetName := clnt.GetURL().String()
	if versionID != "" {
		targetName += " (" + versionID + ")"
	}

	err := clnt.DeleteTags(ctx, versionID)
	if err != nil {
		fatalIf(err, "Unable to remove tags for "+targetName)
		return
	}

	printMsg(tagRemoveMessage{
		Status:    "success",
		Name:      clnt.GetURL().String(),
		VersionID: versionID,
	})
}

func deleteTagsSingle(ctx context.Context, alias, url, versionID string) *probe.Error {
	newClnt, err := newClientFromAlias(alias, url)
	if err != nil {
		return err
	}

	deleteTags(ctx, newClnt, versionID)
	return nil
}

func mainRemoveTag(cliCtx *cli.Context) error {
	ctx, cancelList := context.WithCancel(globalContext)
	defer cancelList()

	console.SetColor("Remove", color.New(color.FgGreen))

	targetURL, versionID, timeRef, withVersions, recursive := parseRemoveTagSyntax(cliCtx)
	if timeRef.IsZero() && withVersions {
		timeRef = time.Now().UTC()
	}

	clnt, pErr := newClient(targetURL)
	fatalIf(pErr, "Unable to initialize target "+targetURL)

	alias, urlStr, _ := mustExpandAlias(targetURL)
	if timeRef.IsZero() && !withVersions && !recursive {
		err := deleteTagsSingle(ctx, alias, urlStr, versionID)
		fatalIf(err.Trace(), "Unable to remove tags on `%s`", targetURL)
		return nil
	}
	for content := range clnt.List(ctx, ListOptions{TimeRef: timeRef, WithOlderVersions: withVersions, Recursive: recursive}) {
		if content.Err != nil {
			fatalIf(content.Err.Trace(), "Unable to list target "+targetURL)
		}

		// Skip if its delete marker
		if content.IsDeleteMarker {
			continue
		}

		if !recursive && alias+getKey(content) != getStandardizedURL(targetURL) {
			break
		}

		err := deleteTagsSingle(ctx, alias, content.URL.String(), content.VersionID)
		if err != nil {
			errorIf(err.Trace(clnt.GetURL().String()), "Invalid URL")
			continue
		}
	}
	return nil
}
