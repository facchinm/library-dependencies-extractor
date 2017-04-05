package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"arduino.cc/builder"
	"arduino.cc/builder/gohasissues"
	"arduino.cc/builder/i18n"
	"arduino.cc/builder/types"
	"arduino.cc/builder/utils"
	"arduino.cc/properties"
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

var buildOptionsFileFlag *string
var hardwareFoldersFlag foldersFlag
var toolsFoldersFlag foldersFlag
var librariesBuiltInFoldersFlag foldersFlag
var librariesFoldersFlag foldersFlag
var customBuildPropertiesFlag propertiesFlag
var fqbnFlag *string
var coreAPIVersionFlag *string
var ideVersionFlag *string
var buildPathFlag *string
var verboseFlag *bool
var quietFlag *bool
var debugLevelFlag *int
var warningsLevelFlag *string
var loggerFlag *string
var vidPidFlag *string

func init() {
	buildOptionsFileFlag = flag.String(FLAG_BUILD_OPTIONS_FILE, "", "Instead of specifying --"+FLAG_HARDWARE+", --"+FLAG_TOOLS+" etc every time, you can load all such options from a file")
	flag.Var(&hardwareFoldersFlag, FLAG_HARDWARE, "Specify a 'hardware' folder. Can be added multiple times for specifying multiple 'hardware' folders")
	flag.Var(&toolsFoldersFlag, FLAG_TOOLS, "Specify a 'tools' folder. Can be added multiple times for specifying multiple 'tools' folders")
	flag.Var(&librariesBuiltInFoldersFlag, FLAG_BUILT_IN_LIBRARIES, "Specify a built-in 'libraries' folder. These are low priority libraries. Can be added multiple times for specifying multiple built-in 'libraries' folders")
	flag.Var(&librariesFoldersFlag, FLAG_LIBRARIES, "Specify a 'libraries' folder. Can be added multiple times for specifying multiple 'libraries' folders")
	flag.Var(&customBuildPropertiesFlag, FLAG_PREFS, "Specify a custom preference. Can be added multiple times for specifying multiple custom preferences")
	fqbnFlag = flag.String(FLAG_FQBN, "", "fully qualified board name")
	coreAPIVersionFlag = flag.String(FLAG_CORE_API_VERSION, "10600", "version of core APIs (used to populate ARDUINO #define)")
	ideVersionFlag = flag.String(FLAG_IDE_VERSION, "10600", "[deprecated] use '"+FLAG_CORE_API_VERSION+"' instead")
	buildPathFlag = flag.String(FLAG_BUILD_PATH, "", "build path")
	verboseFlag = flag.Bool(FLAG_VERBOSE, false, "if 'true' prints lots of stuff")
	quietFlag = flag.Bool(FLAG_QUIET, false, "if 'true' doesn't print any warnings or progress or whatever")
	debugLevelFlag = flag.Int(FLAG_DEBUG_LEVEL, builder.DEFAULT_DEBUG_LEVEL, "Turns on debugging messages. The higher, the chattier")
	warningsLevelFlag = flag.String(FLAG_WARNINGS, "", "Sets warnings level. Available values are '"+FLAG_WARNINGS_NONE+"', '"+FLAG_WARNINGS_DEFAULT+"', '"+FLAG_WARNINGS_MORE+"' and '"+FLAG_WARNINGS_ALL+"'")
	loggerFlag = flag.String(FLAG_LOGGER, FLAG_LOGGER_HUMAN, "Sets type of logger. Available values are '"+FLAG_LOGGER_HUMAN+"', '"+FLAG_LOGGER_MACHINE+"'")
	vidPidFlag = flag.String(FLAG_VID_PID, "", "specify to use vid/pid specific build properties, as defined in boards.txt")
}

func main() {
	flag.Parse()

	ctx := &types.Context{}

	if *buildOptionsFileFlag != "" {
		buildOptions := make(properties.Map)
		if _, err := os.Stat(*buildOptionsFileFlag); err == nil {
			data, err := ioutil.ReadFile(*buildOptionsFileFlag)
			if err != nil {
				printCompleteError(err)
			}
			err = json.Unmarshal(data, &buildOptions)
			if err != nil {
				printCompleteError(err)
			}
		}
		ctx.InjectBuildOptions(buildOptions)
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

	// FLAG_PREFS
	if customBuildProperties, err := toSliceOfUnquoted(customBuildPropertiesFlag); err != nil {
		printCompleteError(err)
	} else if len(customBuildProperties) > 0 {
		ctx.CustomBuildProperties = customBuildProperties
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

	// FLAG_IDE_VERSION
	if ctx.ArduinoAPIVersion == "" {
		// if deprecated "--ideVersionFlag" has been used...
		if *coreAPIVersionFlag == "10600" && *ideVersionFlag != "10600" {
			ctx.ArduinoAPIVersion = *ideVersionFlag
		} else {
			ctx.ArduinoAPIVersion = *coreAPIVersionFlag
		}
	}

	if *warningsLevelFlag != "" {
		ctx.WarningsLevel = *warningsLevelFlag
	}

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

	for i, library := range ctx.Libraries {

		if library.Archs[0] == "*" || utils.SliceContains(library.Archs, "avr") {
			ctx.FQBN = "arduino:avr:micro"
		}
		if strings.Contains(library.Name, "Robot") {
			ctx.FQBN = "arduino:avr:robotControl"
		}
		if strings.Contains(library.Name, "Adafruit") && strings.Contains(library.Name, "Playground") {
			ctx.FQBN = "arduino:avr:circuitplay32u4cat"
		}
		if utils.SliceContains(library.Archs, "sam") {
			ctx.FQBN = "arduino:sam:arduino_due_x_dbg"
		}
		if utils.SliceContains(library.Archs, "samd") {
			ctx.FQBN = "arduino:samd:arduino_zero_edbg"
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

		fmt.Println(sketch)

		sketch += "\nvoid loop(){}\nvoid setup(){}\n"

		ioutil.WriteFile(ctx.SketchLocation, []byte(sketch), 0666)

		err = builder.RunBuilder(ctx)

		os.Remove(tempDir)
		os.RemoveAll(tempDir)
		// clean buildPath/libraries folder (at least)
		//os.Remove(buildPath + "/libraries")

		var deps []string

		for _, dep := range ctx.ImportedLibraries {
			if dep.RealName != library.RealName && !utils.SliceContains(deps, dep.RealName) {
				deps = append(deps, dep.RealName)
			}
		}

		ctx.Libraries[i].Dependencies = deps

		if err != nil {
			err = i18n.WrapError(err)

			fmt.Fprintln(os.Stderr, err)

			if ctx.DebugLevel >= 10 {
				fmt.Fprintln(os.Stderr, err.(*errors.Error).ErrorStack())
			}

			// TODO: try to recompile the library with another architecture

		}

		// search for exaples and compile them
		libraryExamplesPath := filepath.Join(library.Folder, "examples")
		examples, _ := findFilesInFolder(libraryExamplesPath, ".ino", true)
		for _, example := range examples {
			ctx.SketchLocation = example
			ctx.ImportedLibraries = ctx.ImportedLibraries[:0]
			ctx.IncludeFolders = ctx.IncludeFolders[:0]

			fmt.Println("Compiling example: " + example)
			err = builder.RunBuilder(ctx)

			for _, dep := range ctx.ImportedLibraries {
				if dep.RealName != library.RealName && !utils.SliceContains(deps, dep.RealName) {
					deps = append(deps, dep.RealName)
				}
			}

			fmt.Print("library " + library.Name + " depends on ")
			fmt.Print(deps)

			if err != nil {
				fmt.Println(" but failed to compile on " + ctx.FQBN)
			} else {
				fmt.Println("")
			}
		}
	}
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
