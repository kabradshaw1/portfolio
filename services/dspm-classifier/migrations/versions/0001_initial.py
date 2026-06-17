"""initial findings + processed_messages tables.

Revision ID: 0001
Revises:
Create Date: 2026-06-16
"""

from __future__ import annotations

import sqlalchemy as sa
from alembic import op

revision = "0001"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "findings",
        sa.Column("tenant_id", sa.String(128), primary_key=True),
        sa.Column("event_id", sa.String(128), primary_key=True),
        sa.Column("bucket", sa.String(255), nullable=False),
        sa.Column("key", sa.Text, nullable=False),
        sa.Column("sensitivity", sa.Integer, nullable=False),
        sa.Column("categories", sa.ARRAY(sa.String), nullable=False, server_default="{}"),
        sa.Column("match_count", sa.Integer, nullable=False, server_default="0"),
        sa.Column("classified_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("pipeline_version", sa.Integer, nullable=False),
        sa.Column("llm_failed", sa.Boolean, nullable=False, server_default=sa.false()),
        sa.Column("reason", sa.Text, nullable=True),
    )
    op.create_index(
        "ix_findings_tenant_sensitivity_time",
        "findings",
        ["tenant_id", "sensitivity", "classified_at"],
    )

    op.create_table(
        "processed_messages",
        sa.Column("event_id", sa.String(128), primary_key=True),
        sa.Column("tenant_id", sa.String(128), nullable=False),
        sa.Column("processed_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("pipeline_version", sa.Integer, nullable=False),
    )


def downgrade() -> None:
    op.drop_table("processed_messages")
    op.drop_index("ix_findings_tenant_sensitivity_time", table_name="findings")
    op.drop_table("findings")
