/*
Copyright © 2024 Brian Ketelsen bketelsen@gmail.com

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/heimdalr/dag"
	"github.com/u-root/u-root/pkg/ldd"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "uhaul <binary>",
	Args:  cobra.ExactArgs(1),
	Short: "Relocate ELF binaries to a prefix",
	Long: `Relocate ELF binaries to a custom prefix, bringing dynamic
	libraries with you for the ride.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	RunE: uhaulIt,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.uhaul.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().StringP("prefix", "p", "/opt/uhaul", "Installation prefix")
	rootCmd.Flags().StringP("out", "o", "./out", "Output directory")
	rootCmd.Flags().BoolP("clean", "c", true, "Clean output directory")

}

func uhaulIt(cmd *cobra.Command, args []string) error {
	d := dag.NewDAG()

	for i, a := range args {
		slog.Debug("Arguments",
			slog.Int("position", i),
			slog.String("value", a))
	}
	bin := args[0]
	_, err := os.Stat(bin)
	if err != nil {
		fullPath, err := exec.LookPath(bin)
		if err != nil {
			return err
		}
		bin = fullPath

	}
	binary, _ := d.AddVertex(bin)
	err = deps(bin, binary, d)
	if err != nil {
		return err
	}
	fmt.Println(d.String())
	m, err := d.GetDescendants(binary)
	if err != nil {
		return err
	}
	fmt.Printf("%s has %d dynamic dependencies\n", bin, len(m))
	for k, v := range m {
		fmt.Println(k, v)
	}
	prefixDir, err := cmd.Flags().GetString("prefix")
	if err != nil {
		return err
	}
	prefixDir = strings.TrimPrefix(prefixDir, "/")
	slog.Info("prefix", slog.String("value", prefixDir))
	outputDir, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	slog.Info("output", slog.String("directory", outputDir))
	clean, err := cmd.Flags().GetBool("clean")
	if err != nil {
		return err
	}
	// make the directories
	err = makeDirectories(outputDir)
	if err != nil {
		return err
	}
	if clean {
		slog.Info("cleaning", slog.String("directory", outputDir))
		err = cleanDirectory(outputDir)
		if err != nil {
			return err
		}
	}
	// make the prefix directory
	out := filepath.Join(outputDir, prefixDir)
	err = makeDirectories(out)
	if err != nil {
		return err
	}
	binDir := filepath.Join(out, "bin")
	err = makeDirectories(binDir)
	if err != nil {
		return err
	}
	// copy the binary to the output directory
	_, binName := filepath.Split(bin)
	binOut := filepath.Join(binDir, binName)
	err = copyFile(bin, binOut, true)
	if err != nil {
		return err
	}

	// copy the dynamic dependencies to the output directory
	libDir := filepath.Join(out, "lib")
	err = makeDirectories(libDir)
	if err != nil {
		return err
	}
	rpath := "$ORIGIN/../lib"
	cmdStr := fmt.Sprintf("patchelf --set-rpath %s %s", rpath, binOut)
	slog.Info("Setting RPATH", slog.String("cmd", cmdStr))
	pcmd := exec.Command("patchelf", "--set-rpath", rpath, binOut)
	err = pcmd.Run()
	if err != nil {
		return err
	}

	for _, lib := range m {
		libs := lib.(string)
		_, libName := filepath.Split(libs)
		libOut := filepath.Join(libDir, libName)
		err = copyFile(libs, libOut, false)
		if err != nil {
			return err
		}
		rpath := "$ORIGIN"
		cmdStr := fmt.Sprintf("patchelf --set-rpath %s %s", rpath, libOut)
		slog.Info("Setting RPATH", slog.String("cmd", cmdStr))
		pcmd := exec.Command("patchelf", "--set-rpath", rpath, libOut)
		err = pcmd.Run()
		if err != nil {
			return err
		}

	}

	return nil
}

func deps(f string, vertex string, d *dag.DAG) error {
	slog.Info("Traverse", slog.String("vertex", vertex))

	lddDeps, err := ldd.List(f)
	if err != nil {
		return err
	}
	for _, y := range lddDeps {
		slog.Info("Dynamic Links", slog.String("file", f), slog.String("dep", y))
	}

	for _, dep := range lddDeps {
		slog.Info("Vertex", slog.String("dep", dep))
		depName, err := d.AddVertex(dep)
		if err == nil {

			slog.Info("Edge", slog.String("vertex", vertex), slog.String("depName", depName))
			err = d.AddEdge(vertex, depName)
			if err == nil {
				_, err := os.Stat(dep)
				if err != nil {
					return err
				}

				err = deps(dep, depName, d)
				if err != nil {
					return err
				}
			} else {
				continue
			}
		}

	}

	return nil

}

func copyFile(src, dst string, executable bool) error {
	slog.Info("Copying", slog.String("from", src), slog.String("to", dst))

	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	err = os.WriteFile(dst, in, 0755)
	if err != nil {
		return err
	}
	if executable {
		err = os.Chmod(dst, 0755)
		if err != nil {
			return err
		}

	}

	return nil
}

func makeDirectories(p string) error {
	err := os.MkdirAll(p, 0755)
	if err != nil {
		return err
	}
	return nil
}

// delete all the children of a directory
func cleanDirectory(p string) error {

	files, err := os.ReadDir(p)
	if err != nil {
		return err
	}
	for _, f := range files {
		err := os.RemoveAll(f.Name())
		if err != nil {
			return err
		}
	}
	return nil

}
