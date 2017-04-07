package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"arduino.cc/builder"
	"arduino.cc/builder/gohasissues"
	"arduino.cc/builder/i18n"
	"arduino.cc/builder/types"
	"arduino.cc/builder/utils"
	"github.com/go-errors/errors"
	"github.com/masatana/go-textdistance"
)

const VERSION = "1.3.24"

const FLAG_ACTION_COMPILE = "compile"
const FLAG_ACTION_PREPROCESS = "preprocess"
const FLAG_ACTION_DUMP_PREFS = "dump-prefs"
const FLAG_BUILD_OPTIONS_FILE = "build-options-file"
const FLAG_HARDWARE = "hardware"
const FLAG_TOOLS = "tools"
const FLAG_BUILT_IN_LIBRARIES = "built-in-libraries"
const FLAG_LIBRARIES = "libraries"
const FLAG_PREFS = "prefs"
const FLAG_FQBN = "fqbn"
const FLAG_IDE_VERSION = "ide-version"
const FLAG_CORE_API_VERSION = "core-api-version"
const FLAG_BUILD_PATH = "build-path"
const FLAG_VERBOSE = "verbose"
const FLAG_QUIET = "quiet"
const FLAG_DEBUG_LEVEL = "debug-level"
const FLAG_WARNINGS = "warnings"
const FLAG_WARNINGS_NONE = "none"
const FLAG_WARNINGS_DEFAULT = "default"
const FLAG_WARNINGS_MORE = "more"
const FLAG_WARNINGS_ALL = "all"
const FLAG_LOGGER = "logger"
const FLAG_LOGGER_HUMAN = "human"
const FLAG_LOGGER_MACHINE = "machine"
const FLAG_VERSION = "version"
const FLAG_VID_PID = "vid-pid"
const FLAG_JSON = "json"

type foldersFlag []string

func (h *foldersFlag) String() string {
	return fmt.Sprint(*h)
}

func (h *foldersFlag) Set(csv string) error {
	var values []string
	if strings.Contains(csv, string(os.PathListSeparator)) {
		values = strings.Split(csv, string(os.PathListSeparator))
	} else {
		values = strings.Split(csv, ",")
	}

	for _, value := range values {
		value = strings.TrimSpace(value)
		*h = append(*h, value)
	}

	return nil
}

type propertiesFlag []string

func (h *propertiesFlag) String() string {
	return fmt.Sprint(*h)
}

func (h *propertiesFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	*h = append(*h, value)

	return nil
}

var hardwareFoldersFlag foldersFlag
var toolsFoldersFlag foldersFlag
var librariesBuiltInFoldersFlag foldersFlag
var librariesFoldersFlag foldersFlag
var librariesJsonPath *string
var buildPathFlag *string
var verboseFlag *bool
var forceRebuild *bool
var exampleFlag *bool
var quietFlag *bool
var debugLevelFlag *int
var loggerFlag *string

// Output structure used to generate library_index.json file
type indexOutput struct {
	Libraries []indexLibrary `json:"libraries"`
}

// Output structure used to generate library_index.json file
type indexLibrary struct {
	LibraryName     string   `json:"name"`
	Version         string   `json:"version"`
	Author          string   `json:"author"`
	Maintainer      string   `json:"maintainer"`
	License         string   `json:"license,omitempty"`
	Sentence        string   `json:"sentence"`
	Paragraph       string   `json:"paragraph,omitempty"`
	Website         string   `json:"website,omitempty"`
	Category        string   `json:"category,omitempty"`
	Architectures   []string `json:"architectures,omitempty"`
	Types           []string `json:"types,omitempty"`
	Requires        []string `json:"requires, omitempty"`
	URL             string   `json:"url"`
	ArchiveFileName string   `json:"archiveFileName"`
	Size            int64    `json:"size"`
	Checksum        string   `json:"checksum"`

	SupportLevel string `json:"supportLevel,omitempty"`
}

type indexLibrariesAnalyzed struct {
	Exists map[string]bool `json:"name"`
}

func init() {
	flag.Var(&hardwareFoldersFlag, FLAG_HARDWARE, "Specify a 'hardware' folder. Can be added multiple times for specifying multiple 'hardware' folders")
	flag.Var(&toolsFoldersFlag, FLAG_TOOLS, "Specify a 'tools' folder. Can be added multiple times for specifying multiple 'tools' folders")
	flag.Var(&librariesBuiltInFoldersFlag, FLAG_BUILT_IN_LIBRARIES, "Specify a built-in 'libraries' folder. These are low priority libraries. Can be added multiple times for specifying multiple built-in 'libraries' folders")
	flag.Var(&librariesFoldersFlag, FLAG_LIBRARIES, "Specify a 'libraries' folder. Can be added multiple times for specifying multiple 'libraries' folders")
	buildPathFlag = flag.String(FLAG_BUILD_PATH, "", "build path")
	verboseFlag = flag.Bool(FLAG_VERBOSE, false, "if 'true' prints lots of stuff")
	forceRebuild = flag.Bool("force", false, "if 'true' rebuilds all dependencies from scratch")
	exampleFlag = flag.Bool("examples", false, "Also compile all the builtin example")
	quietFlag = flag.Bool(FLAG_QUIET, false, "if 'true' doesn't print any warnings or progress or whatever")
	debugLevelFlag = flag.Int(FLAG_DEBUG_LEVEL, builder.DEFAULT_DEBUG_LEVEL, "Turns on debugging messages. The higher, the chattier")
	loggerFlag = flag.String(FLAG_LOGGER, FLAG_LOGGER_HUMAN, "Sets type of logger. Available values are '"+FLAG_LOGGER_HUMAN+"', '"+FLAG_LOGGER_MACHINE+"'")
	librariesJsonPath = flag.String(FLAG_JSON, "", "specify the starting json file")
}

func main() {
	flag.Parse()

	ctx := &types.Context{}

	// FLAG json
	if *librariesJsonPath == "" {
		fmt.Println("You need to pass the path of a library_index.json")
		os.Exit(1)
	}

	// FLAG_HARDWARE
	if hardwareFolders, err := toSliceOfUnquoted(hardwareFoldersFlag); err != nil {
		printCompleteError(err)
	} else if len(hardwareFolders) > 0 {
		ctx.HardwareFolders = hardwareFolders
	}
	if len(ctx.HardwareFolders) == 0 {
		printErrorMessageAndFlagUsage(errors.New("Parameter '" + FLAG_HARDWARE + "' is mandatory"))
	}

	// FLAG_TOOLS
	if toolsFolders, err := toSliceOfUnquoted(toolsFoldersFlag); err != nil {
		printCompleteError(err)
	} else if len(toolsFolders) > 0 {
		ctx.ToolsFolders = toolsFolders
	}
	if len(ctx.ToolsFolders) == 0 {
		printErrorMessageAndFlagUsage(errors.New("Parameter '" + FLAG_TOOLS + "' is mandatory"))
	}

	// FLAG_LIBRARIES
	if librariesFolders, err := toSliceOfUnquoted(librariesFoldersFlag); err != nil {
		printCompleteError(err)
	} else if len(librariesFolders) > 0 {
		ctx.OtherLibrariesFolders = librariesFolders
	}

	// FLAG_BUILT_IN_LIBRARIES
	if librariesBuiltInFolders, err := toSliceOfUnquoted(librariesBuiltInFoldersFlag); err != nil {
		printCompleteError(err)
	} else if len(librariesBuiltInFolders) > 0 {
		ctx.BuiltInLibrariesFolders = librariesBuiltInFolders
	}

	// FLAG_BUILD_PATH
	buildPath, err := gohasissues.Unquote(*buildPathFlag)
	if err != nil {
		printCompleteError(err)
	}
	if buildPath != "" {
		_, err := os.Stat(buildPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		err = utils.EnsureFolderExists(buildPath)
		if err != nil {
			printCompleteError(err)
		}
	}
	ctx.BuildPath = buildPath

	if *verboseFlag && *quietFlag {
		*verboseFlag = false
		*quietFlag = false
	}

	ctx.Verbose = *verboseFlag

	ctx.ArduinoAPIVersion = "10800"

	if *debugLevelFlag > -1 {
		ctx.DebugLevel = *debugLevelFlag
	}

	if *quietFlag {
		ctx.SetLogger(i18n.NoopLogger{})
	} else if *loggerFlag == FLAG_LOGGER_MACHINE {
		ctx.SetLogger(i18n.MachineLogger{})
	} else {
		ctx.SetLogger(i18n.HumanLogger{})
	}

	// Populate libraries, temporary FQBN
	ctx.FQBN = "arduino:avr:uno"
	builder.RunParseHardwareAndDumpBuildProperties(ctx)

	buildCachePath, _ := ioutil.TempDir("", "core_cache")
	ctx.BuildCachePath = buildCachePath

	var indexJson indexOutput
	var previousRun indexLibrariesAnalyzed
	previousRun.Exists = make(map[string]bool)

	prev, err := ioutil.ReadFile("cached_results.json")
	if err == nil {
		err = json.Unmarshal(prev, &previousRun)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
	}

	dec, _ := ioutil.ReadFile(*librariesJsonPath)

	err = json.Unmarshal(dec, &indexJson)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		tempJsonCTRL, err := json.MarshalIndent(&indexJson, "", "    ")
		if err != nil {
			fmt.Println(err.Error())
		}
		ioutil.WriteFile(*librariesJsonPath, tempJsonCTRL, 0666)
		fmt.Println("Exiting due to CTRL+C")
		os.Exit(2)
	}()

	for _, library := range ctx.Libraries {

		libIndex := indexJsonContains(indexJson.Libraries, library.RealName, library.Version)

		if libIndex == -1 {
			// library not in index, don't create dependency tree
			continue
		}

		if previousRun.Exists[library.Name] == true && *forceRebuild == false {
			// we already have analyzed the dependencies, skip
			// if forceRebuild == true, rebuild them anyway
			continue
		}

		// symlink the folder to a folder called RealName so it gets picked up
		symlinkWithBestName := filepath.Join(library.Folder, "..", library.RealName)
		usingSymlink := false
		if symlinkWithBestName != library.Folder {
			os.Symlink(library.Folder, symlinkWithBestName)
			usingSymlink = true
			fmt.Println("symlinking " + library.Folder + " to " + symlinkWithBestName)
		}

		if library.Archs[0] == "*" || utils.SliceContains(library.Archs, "avr") {
			ctx.FQBN = "arduino:avr:micro"
		}
		if strings.Contains(library.Name, "Robot") {
			if strings.Contains(library.Name, "Control") {
				ctx.FQBN = "arduino:avr:robotControl"
			} else {
				ctx.FQBN = "arduino:avr:robotMotor"
			}
		}
		if strings.Contains(library.Name, "Adafruit") && strings.Contains(library.Name, "Playground") {
			ctx.FQBN = "arduino:avr:circuitplay32u4cat"
		}
		if utils.SliceContains(library.Archs, "sam") {
			ctx.FQBN = "arduino:sam:arduino_due_x_dbg"
		}
		if utils.SliceContains(library.Archs, "samd") {
			ctx.FQBN = "arduino:samd:mkr1000"
		}
		if utils.SliceContains(library.Archs, "arc32") {
			ctx.FQBN = "Intel:arc32:arduino_101"
		}
		if utils.SliceContains(library.Archs, "esp8266") {
			ctx.FQBN = "esp8266:esp8266:nodemcuv2:CpuFrequency=80,UploadSpeed=115200,FlashSize=4M3M"
		}

		//wipe ctx.UsedLibraries
		ctx.ImportedLibraries = ctx.ImportedLibraries[:0]
		ctx.IncludeFolders = ctx.IncludeFolders[:0]

		// create sketch, including all library headers
		tempDir, _ := ioutil.TempDir("", "sketch"+library.Name)

		ctx.SketchLocation, _ = filepath.Abs(tempDir + "/sketch.ino")

		sketch := includeHeadersFromLibraryFolder(library)

		sketch += "\nvoid loop(){}\nvoid setup(){}\n"

		ioutil.WriteFile(ctx.SketchLocation, []byte(sketch), 0666)

		err = builder.RunBuilder(ctx)

		os.Remove(tempDir)
		os.RemoveAll(tempDir)
		// clean buildPath/libraries folder (at least)
		//os.Remove(buildPath + "/libraries")

		var deps []string
		var internal_deps []string

		for _, dep := range ctx.ImportedLibraries {
			if dep.RealName != library.RealName && !utils.SliceContains(deps, dep.RealName) && !utils.SliceContains(internal_deps, dep.RealName) {
				if strings.Contains(dep.Folder, ctx.OtherLibrariesFolders[0]) {
					deps = append(deps, dep.RealName)
				} else {
					internal_deps = append(internal_deps, dep.RealName)
				}
			}
		}

		//ctx.Libraries[i].Dependencies = deps

		fmt.Print("Library " + library.Name + " depends on: ")
		fmt.Print(deps)
		fmt.Print(" provided by lib manager and ")
		fmt.Print(internal_deps)
		fmt.Print(" provided by cores or builtin")

		if err != nil {
			fmt.Println(" but failed to compile on " + ctx.FQBN)
		} else {
			fmt.Println("")
		}

		if *exampleFlag == true {

			// search for examples and compile them
			libraryExamplesPath := filepath.Join(library.Folder, "examples")
			examples, _ := findFilesInFolder(libraryExamplesPath, ".ino", true)

			var errors_examples []string

			for _, example := range examples {
				ctx.SketchLocation = example
				ctx.ImportedLibraries = ctx.ImportedLibraries[:0]
				ctx.IncludeFolders = ctx.IncludeFolders[:0]

				err = builder.RunBuilder(ctx)

				if err != nil {
					errors_examples = append(errors_examples, err.Error())
				}

				for _, dep := range ctx.ImportedLibraries {
					if dep.RealName != library.RealName && !utils.SliceContains(deps, dep.RealName) && !utils.SliceContains(internal_deps, dep.RealName) {
						if strings.Contains(dep.Folder, ctx.OtherLibrariesFolders[0]) {
							deps = append(deps, dep.RealName)
						} else {
							internal_deps = append(internal_deps, dep.RealName)
						}
					}
				}
			}
			fmt.Print("Examples for " + library.Name + " depends on: ")
			fmt.Print(deps)
			fmt.Print(" provided by lib manager and ")
			fmt.Print(internal_deps)
			fmt.Print(" provided by cores or builtin")

			if len(errors_examples) > 0 {
				fmt.Println(" but " + string(len(errors_examples)) + " failed to compile on " + ctx.FQBN)
				// fmt.Println(errors_examples)
			} else {
				fmt.Println("")
			}

		}

		if usingSymlink {
			os.RemoveAll(symlinkWithBestName)
		}

		indexJson.Libraries[libIndex].Requires = deps
		previousRun.Exists[library.Name] = true
	}

	finalJson, err := json.MarshalIndent(&indexJson, "", "    ")
	if err != nil {
		fmt.Println(err.Error())
	}
	ioutil.WriteFile(*librariesJsonPath, finalJson, 0666)

	previousRunJson, err := json.MarshalIndent(&previousRun, "", "    ")
	if err != nil {
		fmt.Println(err.Error())
	}
	ioutil.WriteFile("cached_results.json", previousRunJson, 0666)
}

func indexJsonContains(index []indexLibrary, name, version string) int {
	for idx, lib := range index {
		if lib.LibraryName == name && lib.Version == version {
			return idx
		}
	}
	return -1
}

func includeHeadersFromLibraryFolder(library *types.Library) string {
	headers, _ := findFilesInFolder(library.Folder, ".h", true)
	temp := "\n"
	includedLibs := 0
	for _, header := range headers {
		if textdistance.JaroWinklerDistance(filepath.Base(header), library.Name) > 0.9 {
			temp += "#include \"" + filepath.Base(header) + "\"\n"
			includedLibs += 1
		}
	}
	if includedLibs == 0 && len(headers) > 0 {
		temp += "#include \"" + filepath.Base(headers[0]) + "\"\n"
	}
	return temp
}

func findFilesInFolder(sourcePath string, extension string, recurse bool) ([]string, error) {
	files, err := utils.ReadDirFiltered(sourcePath, utils.FilterFilesWithExtensions(extension))
	if err != nil {
		return nil, i18n.WrapError(err)
	}
	var sources []string
	for _, file := range files {
		sources = append(sources, filepath.Join(sourcePath, file.Name()))
	}

	if recurse {
		folders, err := utils.ReadDirFiltered(sourcePath, utils.FilterDirs)
		if err != nil {
			return nil, i18n.WrapError(err)
		}

		for _, folder := range folders {
			otherSources, err := findFilesInFolder(filepath.Join(sourcePath, folder.Name()), extension, recurse)
			if err != nil {
				return nil, i18n.WrapError(err)
			}
			sources = append(sources, otherSources...)
		}
	}

	return sources, nil
}

func toExitCode(err error) int {
	if exiterr, ok := err.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}
	return 1
}

func toSliceOfUnquoted(value []string) ([]string, error) {
	var values []string
	for _, v := range value {
		v, err := gohasissues.Unquote(v)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, nil
}

func printError(err error, printStackTrace bool) {
	if printStackTrace {
		printCompleteError(err)
	} else {
		printErrorMessageAndFlagUsage(err)
	}
}

func printCompleteError(err error) {
	err = i18n.WrapError(err)
	fmt.Fprintln(os.Stderr, err.(*errors.Error).ErrorStack())
	os.Exit(1)
}

func printErrorMessageAndFlagUsage(err error) {
	fmt.Fprintln(os.Stderr, err)
	flag.Usage()
	os.Exit(1)
}
