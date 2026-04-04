package dev.kylebradshaw.activity.config;

import org.springframework.amqp.core.Binding;
import org.springframework.amqp.core.BindingBuilder;
import org.springframework.amqp.core.Queue;
import org.springframework.amqp.core.TopicExchange;
import org.springframework.amqp.support.converter.Jackson2JsonMessageConverter;
import org.springframework.amqp.support.converter.MessageConverter;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class RabbitConfig {
    public static final String QUEUE_NAME = "activity.queue";
    public static final String EXCHANGE_NAME = "task.events";

    @Bean
    public TopicExchange taskExchange() { return new TopicExchange(EXCHANGE_NAME); }

    @Bean
    public Queue activityQueue() { return new Queue(QUEUE_NAME, true); }

    @Bean
    public Binding binding(Queue activityQueue, TopicExchange taskExchange) {
        return BindingBuilder.bind(activityQueue).to(taskExchange).with("task.*");
    }

    @Bean
    public MessageConverter jsonMessageConverter() { return new Jackson2JsonMessageConverter(); }
}
