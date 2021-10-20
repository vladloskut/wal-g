package mysql

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wal-g/tracelog"
	"github.com/wal-g/wal-g/internal"
	"github.com/wal-g/wal-g/internal/databases/mysql"
)

const (
	backupPushShortDescription = "Creates new backup and pushes it to storage"
	permanentFlag              = "permanent"
	permanentShorthand         = "p"
	addUserDataFlag            = "add-user-data"
)

var (
	// backupPushCmd represents the streamPush command
	backupPushCmd = &cobra.Command{
		Use:   "backup-push",
		Short: backupPushShortDescription,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			internal.RequiredSettings[internal.NameStreamCreateCmd] = true
			internal.RequiredSettings[internal.MysqlDatasourceNameSetting] = true
			err := internal.AssertRequiredSettingsSet()
			tracelog.ErrorLogger.FatalOnError(err)
		},
		Run: func(cmd *cobra.Command, args []string) {
			uploader, err := internal.ConfigureUploader()
			tracelog.ErrorLogger.FatalOnError(err)
			backupCmd, err := internal.GetCommandSetting(internal.NameStreamCreateCmd)
			tracelog.ErrorLogger.FatalOnError(err)

			if userData == "" {
				userData = viper.GetString(internal.SentinelUserDataSetting)
			}

			var partitions = viper.GetInt(internal.StreamSplitterPartitions)
			var blockSize = viper.GetSizeInBytes(internal.StreamSplitterBlockSize)

			mysql.HandleBackupPush(uploader, backupCmd, permanent, userData, partitions, blockSize)
		},
	}
	permanent = false
	userData  = ""
)

func init() {
	cmd.AddCommand(backupPushCmd)

	// TODO: Merge similar backup-push functionality
	// to avoid code duplication in command handlers
	backupPushCmd.Flags().BoolVarP(&permanent, permanentFlag, permanentShorthand,
		false, "Pushes permanent backup")
	backupPushCmd.Flags().StringVar(&userData, addUserDataFlag,
		"", "Write the provided user data to the backup sentinel and metadata files.")
}
