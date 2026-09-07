CREATE TABLE project_reports (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    report_type VARCHAR(20) NOT NULL CHECK (report_type IN ('uat', 'planning', 'testcase')),
    format VARCHAR(10) NOT NULL CHECK (format IN ('xlsx', 'pdf')),
    file_path VARCHAR(500) NOT NULL,
    generated_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_project_reports_project
    ON project_reports(project_id, created_at DESC, id DESC);

CREATE TABLE project_report_items (
    id UUID PRIMARY KEY,
    project_report_id UUID NOT NULL REFERENCES project_reports(id) ON DELETE CASCADE,
    source_ref VARCHAR(255) NOT NULL DEFAULT '',
    title VARCHAR(500) NOT NULL,
    detail TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT '',
    sequence_no INT NOT NULL
);

CREATE INDEX idx_project_report_items_report
    ON project_report_items(project_report_id, sequence_no);
