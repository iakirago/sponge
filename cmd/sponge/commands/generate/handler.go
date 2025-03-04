package generate

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/zhufuyi/sponge/pkg/replacer"
	"github.com/zhufuyi/sponge/pkg/sql2code"
	"github.com/zhufuyi/sponge/pkg/sql2code/parser"

	"github.com/spf13/cobra"
)

// HandlerCommand generate handler codes
func HandlerCommand() *cobra.Command {
	var (
		moduleName string // module name for go.mod
		outPath    string // output directory
		dbTables   string // table names

		sqlArgs = sql2code.Args{
			Package:  "model",
			JSONTag:  true,
			GormType: true,
		}
	)

	cmd := &cobra.Command{
		Use:   "handler",
		Short: "Generate handler codes based on mysql table",
		Long: `generate handler codes based on mysql table.

Examples:
  # generate handler codes and embed 'gorm.model' struct.
  sponge web handler --module-name=yourModuleName --db-dsn=root:123456@(192.168.3.37:3306)/test --db-table=user

  # generate handler codes with multiple table names.
  sponge web handler --module-name=yourModuleName --db-dsn=root:123456@(192.168.3.37:3306)/test --db-table=t1,t2

  # generate handler codes, structure fields correspond to the column names of the table.
  sponge web handler --module-name=yourModuleName --db-dsn=root:123456@(192.168.3.37:3306)/test --db-table=user --embed=false

  # generate handler codes and specify the server directory, Note: code generation will be canceled when the latest generated file already exists.
  sponge web handler --db-dsn=root:123456@(192.168.3.37:3306)/test --db-table=user --out=./yourServerDir
`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			mdName, _ := getNamesFromOutDir(outPath)
			if mdName != "" {
				moduleName = mdName
			} else if moduleName == "" {
				return errors.New(`required flag(s) "module-name" not set, use "sponge web handler -h" for help`)
			}

			tableNames := strings.Split(dbTables, ",")
			for _, tableName := range tableNames {
				if tableName == "" {
					continue
				}

				sqlArgs.DBTable = tableName
				codes, err := sql2code.Generate(&sqlArgs)
				if err != nil {
					return err
				}

				outDir, err := runGenHandlerCommand(moduleName, codes, outPath)
				if err != nil {
					return err
				}
				if outPath == "" {
					outPath = outDir
				}
			}

			fmt.Printf("generate 'handler' codes successfully, out = %s\n\n", outPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&moduleName, "module-name", "m", "", "module-name is the name of the module in the 'go.mod' file")
	//_ = cmd.MarkFlagRequired("module-name")
	cmd.Flags().StringVarP(&sqlArgs.DBDsn, "db-dsn", "d", "", "db content addr, e.g. user:password@(host:port)/database")
	_ = cmd.MarkFlagRequired("db-dsn")
	cmd.Flags().StringVarP(&dbTables, "db-table", "t", "", "table name, multiple names separated by commas")
	_ = cmd.MarkFlagRequired("db-table")
	cmd.Flags().BoolVarP(&sqlArgs.IsEmbed, "embed", "e", true, "whether to embed 'gorm.Model' struct")

	cmd.Flags().StringVarP(&outPath, "out", "o", "", "output directory, default is ./handler_<time>, "+
		"if you specify the directory where the http or rcp server generated by sponge, the module-name flag can be ignored")

	return cmd
}

func runGenHandlerCommand(moduleName string, codes map[string]string, outPath string) (string, error) {
	subTplName := "handler"
	r := Replacers[TplNameSponge]
	if r == nil {
		return "", errors.New("replacer is nil")
	}

	// setting up template information
	subDirs := []string{"internal/model", "internal/cache", "internal/dao",
		"internal/ecode", "internal/handler", "internal/routers", "internal/types"} // only the specified subdirectory is processed, if empty or no subdirectory is specified, it means all files
	ignoreDirs := []string{} // specify the directory in the subdirectory where processing is ignored
	ignoreFiles := []string{ // specify the files in the subdirectory to be ignored for processing
		"systemCode_http.go", "systemCode_rpc.go", "userExample_rpc.go", // internal/ecode
		"init.go", "init_test.go", // internal/model
		"routers.go", "routers_test.go", "routers_pbExample.go", "routers_pbExample_test.go", "userExample_service.pb.go", // internal/routers
		"swagger_types.go", // internal/types
	}

	r.SetSubDirsAndFiles(subDirs)
	r.SetIgnoreSubDirs(ignoreDirs...)
	r.SetIgnoreSubFiles(ignoreFiles...)
	fields := addHandlerFields(moduleName, r, codes)
	r.SetReplacementFields(fields)
	_ = r.SetOutputDir(outPath, subTplName)
	if err := r.SaveFiles(); err != nil {
		return "", err
	}

	return r.GetOutputDir(), nil
}

func addHandlerFields(moduleName string, r replacer.Replacer, codes map[string]string) []replacer.Field {
	var fields []replacer.Field

	fields = append(fields, deleteFieldsMark(r, modelFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, daoFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, daoTestFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, handlerFile, startMark, endMark)...)
	fields = append(fields, deleteFieldsMark(r, handlerTestFile, startMark, endMark)...)
	fields = append(fields, []replacer.Field{
		{ // replace the contents of the model/userExample.go file
			Old: modelFileMark,
			New: codes[parser.CodeTypeModel],
		},
		{ // replace the contents of the dao/userExample.go file
			Old: daoFileMark,
			New: codes[parser.CodeTypeDAO],
		},
		{ // replace the contents of the handler/userExample.go file
			Old: handlerFileMark,
			New: adjustmentOfIDType(codes[parser.CodeTypeHandler]),
		},
		{
			Old: selfPackageName + "/" + r.GetSourcePath(),
			New: moduleName,
		},
		{
			Old: "github.com/zhufuyi/sponge",
			New: moduleName,
		},
		{
			Old: "userExampleNO       = 1",
			New: fmt.Sprintf("userExampleNO = %d", rand.Intn(100)),
		},
		{
			Old: moduleName + "/pkg",
			New: "github.com/zhufuyi/sponge/pkg",
		},
		{
			Old:             "UserExample",
			New:             codes[parser.TableName],
			IsCaseSensitive: true,
		},
	}...)

	return fields
}
