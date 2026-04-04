package dev.kylebradshaw.gateway.config;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.client.RestClient;

@Configuration
public class RestClientConfig {

    @Bean("taskRestClient")
    public RestClient taskServiceClient(@Value("${app.services.task-url}") String taskUrl) {
        return RestClient.builder()
                .baseUrl(taskUrl)
                .build();
    }

    @Bean("activityRestClient")
    public RestClient activityServiceClient(@Value("${app.services.activity-url}") String activityUrl) {
        return RestClient.builder()
                .baseUrl(activityUrl)
                .build();
    }

    @Bean("notificationRestClient")
    public RestClient notificationServiceClient(@Value("${app.services.notification-url}") String notificationUrl) {
        return RestClient.builder()
                .baseUrl(notificationUrl)
                .build();
    }
}
