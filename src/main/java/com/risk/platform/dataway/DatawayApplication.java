package com.risk.platform.dataway;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.ConfigurationPropertiesScan;

/**
 * 数据网关：业务服务访问数据库的唯一出口。
 *
 * <p>业务服务不持 JDBC 驱动、不写裸 SQL（红线 RL-005），所有读写都经本服务：
 * 只读查询必须通过 SqlGuard（单条 SELECT + 表白名单），写入必须命中登记模板。
 */
@SpringBootApplication
@ConfigurationPropertiesScan
public class DatawayApplication {

    public static void main(String[] args) {
        SpringApplication.run(DatawayApplication.class, args);
    }
}
