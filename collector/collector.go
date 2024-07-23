package collector

import (
	"context"
	"dameng_exporter/db"
	"dameng_exporter/logger"
	"database/sql"
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"sync"
)

var (
	collectors  []prometheus.Collector
	registerMux sync.Mutex
	//timeout     = 5 * time.Second
)

const (
	dmdbms_tablespace_file_total_info string = "dmdbms_tablespace_file_total_info"
	dmdbms_tablespace_file_free_info  string = "dmdbms_tablespace_file_free_info"
	dmdbms_start_time_info            string = "dmdbms_start_time_info"
	dmdbms_status_info                string = "dmdbms_status_info"
	dmdbms_mode_info                  string = "dmdbms_mode_info"
	dmdbms_trx_info                   string = "dmdbms_trx_info"
	dmdbms_dead_lock_num_info         string = "dmdbms_dead_lock_num_info"
	dmdbms_thread_num_info            string = "dmdbms_thread_num_info"
	dmdbms_switching_occurs           string = "dmdbms_switching_occurs"
	dmdbms_db_status_occurs           string = "dmdbms_db_status_occurs"
	dmdbms_joblog_error_num           string = "dmdbms_joblog_error_num"
	dmdbms_joblog_error_alarm         string = "dmdbms_joblog_error_alarm"
	dmdbms_start_day                  string = "dmdbms_start_day"
	dmdbms_waiting_session            string = "dmdbms_waiting_session"
	dmdbms_connect_session            string = "dmdbms_connect_session"
	dmdbms_tps_count                  string = "dmdbms_tps_count"
	dmdbms_rapply_sys_task_num        string = "dmdbms_rapply_sys_task_num"
	dmdbms_rapply_sys_task_mem_used   string = "dmdbms_rapply_sys_task_mem_used"
	dmdbms_instance_log_error_info    string = "dmdbms_instance_log_error_info"
)

/*func init() {
	var err error
	db, err = sql.Open("mysql", config.GetDSN())
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
}
*/

// MetricCollector 接口
type MetricCollector interface {
	Describe(ch chan<- *prometheus.Desc)
	Collect(ch chan<- prometheus.Metric)
}

// 注册所有的收集器
func RegisterCollectors(reg *prometheus.Registry, registerHostMetrics, registerDatabaseMetrics, registerDmhsMetrics bool) {
	registerMux.Lock()
	defer registerMux.Unlock()

	if registerHostMetrics {
		//collectors = append(collectors, NewExampleCounterCollector())
	}
	if registerDatabaseMetrics {
		collectors = append(collectors, NewDBSessionsCollector(db.DBPool))
		collectors = append(collectors, NewTablespaceFileInfoCollector(db.DBPool))
		collectors = append(collectors, NewDBInstanceRunningInfoCollector(db.DBPool))

	}
	if registerDmhsMetrics {
		// 添加中间件指标收集器
		// collectors = append(collectors, NewMiddlewareCollector())
	}

	for _, collector := range collectors {
		reg.MustRegister(collector)
	}
}

// 卸载所有的收集器
func UnregisterCollectors(reg *prometheus.Registry) {
	registerMux.Lock()
	defer registerMux.Unlock()

	for _, collector := range collectors {
		reg.Unregister(collector)
	}
	collectors = nil
}

// 封装数据库连接检查逻辑
func checkDBConnection(db *sql.DB) error {
	if err := db.Ping(); err != nil {
		logger.Logger.Error("Database connection is not available", zap.Error(err))
		return err
	}
	return nil
}

// 封装通用的错误处理逻辑
func handleDbQueryError(err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		logger.Logger.Error("Query timed out", zap.Error(err))
	} else {
		logger.Logger.Error("Error querying database", zap.Error(err))
	}
}