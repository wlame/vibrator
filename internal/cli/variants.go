package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/docker"
)

// variantLabel is the label key we filter on when listing vibrator-
// managed images and containers. Set on every `docker build` via the
// Dockerfile (LABEL vibrator.harness=...) and on every `docker run` via
// the orchestrator (--label vibrator.managed=true).
const variantLabel = "vibrator.managed=true"

// variantsCmd manages the set of locally-built vibrator images and their
// associated containers.
var variantsCmd = &cobra.Command{
	Use:   "variants",
	Short: "List or prune locally-built vibrator image variants and containers",
}

var variantsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List vibrator-managed images and containers",
	RunE:  runVariantsList,
}

// variantsPruneFlags holds the prune flag state.
type variantsPruneFlags struct {
	containers bool
	images     bool
	force      bool
}

var variantsPruneFlagsState variantsPruneFlags

var variantsPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove stopped vibrator containers and unused images",
	Long: `Removes vibrator-managed objects.

By default removes stopped containers and dangling (untagged) images.
Use --containers/--images flags to scope; --force to also remove
running containers.`,
	RunE: runVariantsPrune,
}

func init() {
	variantsPruneCmd.Flags().BoolVar(&variantsPruneFlagsState.containers, "containers", false,
		"Remove containers only (skips images).")
	variantsPruneCmd.Flags().BoolVar(&variantsPruneFlagsState.images, "images", false,
		"Remove images only (skips containers).")
	variantsPruneCmd.Flags().BoolVar(&variantsPruneFlagsState.force, "force", false,
		"Force-remove running containers (kill + rm).")
	variantsCmd.AddCommand(variantsListCmd)
	variantsCmd.AddCommand(variantsPruneCmd)
	rootCmd.AddCommand(variantsCmd)
}

// runVariantsList shells out to `docker images --filter label=...` and
// `docker ps -a --filter label=...` and prints a tabular summary.
func runVariantsList(cmd *cobra.Command, _ []string) error {
	dc, err := docker.NewCLIClient()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	c := newColors(!isTerminal(out))

	images, err := dc.ListImages(context.Background(), "vibrator.harness")
	if err != nil {
		return err
	}
	containers, err := dc.ListContainers(context.Background(), variantLabel)
	if err != nil {
		return err
	}

	// --- Images ---
	fmt.Fprintf(out, "%sImages (%d managed)%s\n", c.bold, len(images), c.reset)
	fmt.Fprintln(out, strings.Repeat("-", 30))
	if len(images) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		sort.Slice(images, func(i, j int) bool { return images[i].Tag < images[j].Tag })
		for _, im := range images {
			fmt.Fprintf(out, "  %s\n", im.Tag)
			fmt.Fprintf(out, "    harness:  %s\n", im.Labels["vibrator.harness"])
			fmt.Fprintf(out, "    profile:  %s\n", im.Labels["vibrator.profile"])
			fmt.Fprintf(out, "    features: %s\n", or(im.Labels["vibrator.features"], "(none)"))
			fmt.Fprintf(out, "    catalog:  %s\n", or(im.Labels["vibrator.catalog"], "(none)"))
			fmt.Fprintf(out, "    %ssize:     %s, built %s%s\n", c.dim, im.SizeHuman, im.CreatedAt, c.reset)
		}
	}

	// --- Containers ---
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%sContainers (%d managed)%s\n", c.bold, len(containers), c.reset)
	fmt.Fprintln(out, strings.Repeat("-", 30))
	if len(containers) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
		for _, ct := range containers {
			marker := c.dim
			if strings.HasPrefix(ct.Status, "Up") {
				marker = c.green
			}
			fmt.Fprintf(out, "  %s%s%s  %s%s%s\n", marker, ct.Name, c.reset, c.dim, ct.Status, c.reset)
			fmt.Fprintf(out, "    image:     %s\n", ct.Image)
			fmt.Fprintf(out, "    workspace: %s\n", or(ct.Labels["vibrator.path"], "(unset)"))
		}
	}
	return nil
}

// runVariantsPrune deletes vibrator-managed objects. By default
// removes only stopped containers + dangling images.
func runVariantsPrune(cmd *cobra.Command, _ []string) error {
	dc, err := docker.NewCLIClient()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	c := newColors(!isTerminal(out))

	// Default = both, unless one is explicitly selected.
	doContainers := variantsPruneFlagsState.containers || !variantsPruneFlagsState.images
	doImages := variantsPruneFlagsState.images || !variantsPruneFlagsState.containers

	removedContainers, removedImages := 0, 0

	if doContainers {
		ctList, err := dc.ListContainers(context.Background(), variantLabel)
		if err != nil {
			return err
		}
		for _, ct := range ctList {
			running := strings.HasPrefix(ct.Status, "Up")
			if running && !variantsPruneFlagsState.force {
				fmt.Fprintf(out, "  %s↷ skipping running container %s (use --force to kill)%s\n",
					c.dim, ct.Name, c.reset)
				continue
			}
			if err := dc.Remove(context.Background(), docker.RemoveContainer, ct.ID,
				variantsPruneFlagsState.force); err != nil {
				fmt.Fprintf(out, "  %s✗ failed to remove %s: %v%s\n", c.red, ct.Name, err, c.reset)
				continue
			}
			fmt.Fprintf(out, "  %s✓ removed container %s%s\n", c.green, ct.Name, c.reset)
			removedContainers++
		}
	}

	if doImages {
		imgs, err := dc.ListImages(context.Background(), "vibrator.harness")
		if err != nil {
			return err
		}
		for _, im := range imgs {
			if err := dc.Remove(context.Background(), docker.RemoveImage, im.Tag, false); err != nil {
				fmt.Fprintf(out, "  %s↷ skipping in-use image %s%s\n", c.dim, im.Tag, c.reset)
				continue
			}
			fmt.Fprintf(out, "  %s✓ removed image %s%s\n", c.green, im.Tag, c.reset)
			removedImages++
		}
	}

	fmt.Fprintf(out, "\n%sDone:%s removed %d container(s), %d image(s)\n",
		c.bold, c.reset, removedContainers, removedImages)
	return nil
}
