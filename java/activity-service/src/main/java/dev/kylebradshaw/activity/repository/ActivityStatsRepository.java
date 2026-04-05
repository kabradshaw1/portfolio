package dev.kylebradshaw.activity.repository;

import dev.kylebradshaw.activity.dto.EventTypeCountRow;
import dev.kylebradshaw.activity.dto.WeeklyActivityRow;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.List;
import org.bson.Document;
import org.springframework.data.domain.Sort;
import org.springframework.data.mongodb.core.MongoTemplate;
import org.springframework.data.mongodb.core.aggregation.Aggregation;
import org.springframework.data.mongodb.core.aggregation.AggregationExpression;
import org.springframework.data.mongodb.core.aggregation.AggregationResults;
import org.springframework.data.mongodb.core.query.Criteria;
import org.springframework.data.mongodb.core.query.Query;
import org.springframework.stereotype.Repository;

@Repository
public class ActivityStatsRepository {

    private final MongoTemplate mongo;

    public ActivityStatsRepository(MongoTemplate mongo) {
        this.mongo = mongo;
    }

    public int countEvents(String projectId) {
        Query query = new Query(Criteria.where("projectId").is(projectId));
        return (int) mongo.count(query, "activity_events");
    }

    public List<EventTypeCountRow> countByEventType(String projectId) {
        Aggregation agg =
                Aggregation.newAggregation(
                        Aggregation.match(Criteria.where("projectId").is(projectId)),
                        Aggregation.group("eventType").count().as("count"),
                        Aggregation.sort(Sort.Direction.DESC, "count"));

        AggregationResults<Document> results =
                mongo.aggregate(agg, "activity_events", Document.class);
        return results.getMappedResults().stream()
                .map(doc -> new EventTypeCountRow(doc.getString("_id"), doc.getInteger("count")))
                .toList();
    }

    public int countComments(String projectId) {
        Aggregation agg =
                Aggregation.newAggregation(
                        Aggregation.match(Criteria.where("projectId").is(projectId)),
                        Aggregation.group("taskId"));
        AggregationResults<Document> taskIds =
                mongo.aggregate(agg, "activity_events", Document.class);
        List<String> ids =
                taskIds.getMappedResults().stream()
                        .map(doc -> doc.getString("_id"))
                        .toList();
        if (ids.isEmpty()) {
            return 0;
        }
        Query query = new Query(Criteria.where("taskId").in(ids));
        return (int) mongo.count(query, "comments");
    }

    public int countActiveContributors(String projectId) {
        Aggregation agg =
                Aggregation.newAggregation(
                        Aggregation.match(Criteria.where("projectId").is(projectId)),
                        Aggregation.group("actorId"));
        return mongo.aggregate(agg, "activity_events", Document.class)
                .getMappedResults()
                .size();
    }

    public List<WeeklyActivityRow> weeklyActivity(String projectId, int weeks) {
        Instant cutoff = Instant.now().minus(weeks * 7L, ChronoUnit.DAYS);

        Aggregation agg =
                Aggregation.newAggregation(
                        Aggregation.match(
                                Criteria.where("projectId")
                                        .is(projectId)
                                        .and("timestamp")
                                        .gte(cutoff)),
                        Aggregation.project()
                                .and(isoDatePart("$isoWeek"))
                                .as("isoWeek")
                                .and(isoDatePart("$isoWeekYear"))
                                .as("isoYear"),
                        Aggregation.group("isoYear", "isoWeek").count().as("events"),
                        Aggregation.sort(Sort.Direction.DESC, "_id.isoYear", "_id.isoWeek"));

        AggregationResults<Document> results =
                mongo.aggregate(agg, "activity_events", Document.class);
        return results.getMappedResults().stream()
                .map(
                        doc -> {
                            Document id = doc.get("_id", Document.class);
                            String week =
                                    String.format(
                                            "%d-W%02d",
                                            id.getInteger("isoYear"),
                                            id.getInteger("isoWeek"));
                            return new WeeklyActivityRow(week, doc.getInteger("events"), 0);
                        })
                .toList();
    }

    private static AggregationExpression isoDatePart(String operator) {
        return context -> new Document(operator, "$timestamp");
    }
}
