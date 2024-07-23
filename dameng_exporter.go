package main

import (
	"dameng_exporter/collector"
	"dameng_exporter/config"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"fmt"
	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"net/http"
	"os"
	"path/filepath"
)

var (
	registerHostMetrics     bool = true //主机类的数据库指标 例如进程是否存活
	registerDatabaseMetrics bool = true
	registerDmhsMetrics     bool = true //DMHS的相关指标
)

/*if registerHostMetrics || registerDatabaseMetrics || registerMiddlewareMetrics {
// 注册自定义指标
collector.RegisterCollectors(registerHostMetrics, registerDatabaseMetrics, registerMiddlewareMetrics)
} else {
// 卸载所有注册器
collector.UnregisterCollectors()
}
*/

func main() {

	var (
		configFile = kingpin.Flag("configFile", "Path to configuration file").Default(config.DefaultConfig.ConfigFile).String()
		listenAddr = kingpin.Flag("listenAddress", "Address to listen on").Default(config.DefaultConfig.ListenAddress).String()
		metricPath = kingpin.Flag("metricPath", "Path for metrics").Default(config.DefaultConfig.MetricPath).String()
		dbHost     = kingpin.Flag("dbHost", "Database Host").Default(config.DefaultConfig.DbHost).String()
		dbUser     = kingpin.Flag("dbUser", "Database user").Default(config.DefaultConfig.DbUser).String()
		dbPwd      = kingpin.Flag("dbPwd", "Database password").Default(config.DefaultConfig.DbPwd).String()
		//queryTimeout = kingpin.Flag("queryTimeout", "Timeout for queries").Default(config.DefaultConfig.QueryTimeout.String()).Duration()
		queryTimeout = kingpin.Flag("queryTimeout", "Timeout for queries").Default(fmt.Sprint(config.DefaultConfig.QueryTimeout)).Int()
		maxOpenConns = kingpin.Flag("maxOpenConns", "Maximum open connections (number)").Default(fmt.Sprint(config.DefaultConfig.MaxOpenConns)).Int()
		maxIdleConns = kingpin.Flag("maxIdleConns", "Maximum idle connections (number)").Default(fmt.Sprint(config.DefaultConfig.MaxIdleConns)).Int()
		connMaxLife  = kingpin.Flag("connMaxLifetime", "Connection maximum lifetime (Minute)").Default(fmt.Sprint(config.DefaultConfig.ConnMaxLifetime)).Int()

		logMaxSize    = kingpin.Flag("logMaxSize", "Maximum log file size(MB)").Default(fmt.Sprint(config.DefaultConfig.LogMaxSize)).Int()
		logMaxBackups = kingpin.Flag("logMaxBackups", "Maximum log file backups (number)").Default(fmt.Sprint(config.DefaultConfig.LogMaxBackups)).Int()
		logMaxAge     = kingpin.Flag("logMaxAge", "Maximum log file age (Day)").Default(fmt.Sprint(config.DefaultConfig.LogMaxAge)).Int()

		encryptPwd      = kingpin.Flag("encryptPwd", "Password to encrypt and exit").Default("").String()
		encodeConfigPwd = kingpin.Flag("encodeConfigPwd", "Encode the password in the config file").Default("true").Bool()
		Version         = "0.0.1"
	)
	kingpin.Parse()
	//加密密码口令返回
	if execEncryptPwdCmd(encryptPwd) {
		return
	}
	//合并配置文件属性
	mergeConfigParam(configFile, listenAddr, metricPath, queryTimeout, maxIdleConns, maxOpenConns, connMaxLife, logMaxSize, logMaxBackups, logMaxAge, dbUser, dbPwd, dbHost)

	logger.InitLogger() // 初始化全局日志记录器
	defer logger.Sync() // 确保程序退出前同步日志

	//对配置文件密码进行加密
	EncryptPasswordConfig(configFile, encodeConfigPwd)

	// 创建一个新的注册器
	reg := prometheus.NewRegistry()

	//新建数据库连接
	// DSN (Data Source Name) format: user/password@host:port/service_name
	dsn := buildDSN(config.GlobalConfig.DbUser, config.GlobalConfig.DbPwd, config.GlobalConfig.DbHost)
	// 获取主机名
	hostname, host_err := os.Hostname()
	if host_err != nil {
		logger.Logger.Fatalf("Failed to get hostname", zap.Error(host_err))
	}
	config.SetHostName(hostname)

	// 初始化数据库连接池
	err := db.InitDBPool(dsn)
	if err != nil {
		logger.Logger.Fatalf("Failed to initialize database pool: %v", zap.Error(err))
	}
	defer db.CloseDBPool() // 关闭数据库连接池

	//注册指标
	collector.RegisterCollectors(reg, registerHostMetrics, registerDatabaseMetrics, registerDmhsMetrics)
	logger.Logger.Info("Starting dmdb_exporter " + Version)
	//设置metric路径
	http.Handle(config.GlobalConfig.MetricPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	//设置端口号
	if err := http.ListenAndServe(config.GlobalConfig.ListenAddress, nil); err != nil {
		logger.Logger.Errorf("Error occur when start server %v", zap.Error(err))
	}

}

func EncryptPasswordConfig(configFile *string, encodeConfigPwd *bool) /*error*/ {
	//获取全路径
	configFilePath, err := filepath.Abs(*configFile)
	if err != nil {
		logger.Logger.Fatalf("Error determining absolute path for config file: %v", err)
	}
	// 修改密码以及配置文件
	if *encodeConfigPwd && config.GlobalConfig.DbPwd != "" && fileExists(configFilePath) {
		config.GlobalConfig.DbPwd = config.EncryptPassword(config.GlobalConfig.DbPwd)
		err = config.UpdateConfigPassword(configFilePath, config.GlobalConfig.DbPwd)
		if err != nil {
			logger.Logger.Fatalf("Error saving encoded password to config file: %v", err)
		}
		logger.Logger.Infof("Password in config file has been encoded")
		//return
	}
	//return err
}

// 把 默认参数以及配置文件进行合并
func mergeConfigParam(configFile *string, listenAddr *string, metricPath *string, queryTimeout, maxIdleConns *int, maxOpenConns, connMaxLife *int, logMaxSize *int, logMaxBackups *int, logMaxAge *int, dbUser *string, dbPwd *string, dbHost *string) /*(config.Config, error)*/ {
	//读取预先设定的配置文件
	glocal_config, err := config.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("Error loading config file: %v", err)
	}
	// 对默认值以及配置文件的参数进行合并覆盖
	applyConfigFromFlags(&glocal_config, listenAddr, metricPath, queryTimeout, maxIdleConns, maxOpenConns, connMaxLife, logMaxSize, logMaxBackups, logMaxAge, dbUser, dbPwd, dbHost)

	config.GlobalConfig = &glocal_config
	//return glocal_config, err
}

func execEncryptPwdCmd(encryptPwd *string) bool {
	//命令行参数，对密码加密并返回结果
	if *encryptPwd != "" {
		encryptedPwd := config.EncryptPassword(*encryptPwd)
		fmt.Printf("Encrypted Password: %s\n", encryptedPwd)
		return true
	}
	return false
}

func buildDSN(user, password, host string) string {
	//dsn := "dm://SYSDBA:SYSDBA@120.53.103.235:5236?autoCommit=true"
	escapedPwd, _ := config.DecryptPassword(password)
	return fmt.Sprintf("dm://%s:%s@%s?autoCommit=true", user, escapedPwd, host)
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false /*, nil*/
	}
	return true /*, err*/
}

func applyConfigFromFlags(glocal_config *config.Config, listenAddr, metricPath *string, queryTimeout, maxIdleConns, maxOpenConns, connMaxLife *int, logMaxSize, logMaxBackups, logMaxAge *int, dbUser, dbPwd, dbHost *string) {
	if listenAddr != nil && *listenAddr != config.DefaultConfig.ListenAddress {
		glocal_config.ListenAddress = *listenAddr
	}
	if metricPath != nil && *metricPath != config.DefaultConfig.MetricPath {
		glocal_config.MetricPath = *metricPath
	}
	if queryTimeout != nil && *queryTimeout != config.DefaultConfig.QueryTimeout {
		glocal_config.QueryTimeout = *queryTimeout
	}
	if maxIdleConns != nil && *maxIdleConns != config.DefaultConfig.MaxIdleConns {
		glocal_config.MaxIdleConns = *maxIdleConns
	}
	if maxOpenConns != nil && *maxOpenConns != config.DefaultConfig.MaxOpenConns {
		glocal_config.MaxOpenConns = *maxOpenConns
	}
	if connMaxLife != nil && *connMaxLife != config.DefaultConfig.ConnMaxLifetime {
		glocal_config.ConnMaxLifetime = *connMaxLife
	}
	if logMaxSize != nil && *logMaxSize != config.DefaultConfig.LogMaxSize {
		glocal_config.LogMaxSize = *logMaxSize
	}
	if logMaxBackups != nil && *logMaxBackups != config.DefaultConfig.LogMaxBackups {
		glocal_config.LogMaxBackups = *logMaxBackups
	}
	if logMaxAge != nil && *logMaxAge != config.DefaultConfig.LogMaxAge {
		glocal_config.LogMaxAge = *logMaxAge
	}
	if dbUser != nil && *dbUser != config.DefaultConfig.DbUser {
		glocal_config.DbUser = *dbUser
	}
	if dbPwd != nil && *dbPwd != config.DefaultConfig.DbPwd {
		glocal_config.DbPwd = *dbPwd
	}
	if dbHost != nil && *dbHost != config.DefaultConfig.DbHost {
		glocal_config.DbHost = *dbHost
	}

}