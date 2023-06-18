use std::{collections::HashMap, sync::Arc};

use anyhow::{anyhow, Context};
use flow_rs::FlowJob;
use peer_cursor::QueryExecutor;
use peer_postgres::PostgresQueryExecutor;
use prost::Message;
use pt::peers::{peer::Config, DbType, Peer};
use tokio_postgres::{types, Client};

mod embedded {
    use refinery::embed_migrations;
    embed_migrations!("migrations");
}

pub struct Catalog {
    pg: Box<Client>,
    executor: Arc<Box<dyn QueryExecutor>>,
}

async fn run_migrations(client: &mut Client) -> anyhow::Result<()> {
    let migration_report = embedded::migrations::runner()
        .run_async(client)
        .await
        .context("Failed to run migrations")?;
    for migration in migration_report.applied_migrations() {
        tracing::info!(
            "Migration Applied -  Name: {}, Version: {}",
            migration.name(),
            migration.version()
        );
    }
    Ok(())
}

#[derive(Debug, Clone)]
pub struct CatalogConfig {
    pub host: String,
    pub port: u16,
    pub user: String,
    pub password: String,
    pub database: String,
}

impl CatalogConfig {
    pub fn new(host: String, port: u16, user: String, password: String, database: String) -> Self {
        Self {
            host,
            port,
            user,
            password,
            database,
        }
    }

    // convert catalog config to PostgresConfig
    pub fn to_postgres_config(&self) -> pt::peers::PostgresConfig {
        pt::peers::PostgresConfig {
            host: self.host.clone(),
            port: self.port as u32,
            user: self.user.clone(),
            password: self.password.clone(),
            database: self.database.clone(),
        }
    }

    // Get the connection string for this catalog configuration
    pub fn get_connection_string(&self) -> String {
        let mut connection_string = String::new();
        connection_string.push_str("host=");
        connection_string.push_str(&self.host);
        connection_string.push_str(" port=");
        connection_string.push_str(&self.port.to_string());
        connection_string.push_str(" user=");
        connection_string.push_str(&self.user);
        if !self.password.is_empty() {
            connection_string.push_str(" password=");
            connection_string.push_str(&self.password);
        }
        connection_string.push_str(" dbname=");
        connection_string.push_str(&self.database);
        connection_string
    }
}

impl Catalog {
    pub async fn new(catalog_config: &CatalogConfig) -> anyhow::Result<Self> {
        let connection_string = catalog_config.get_connection_string();

        let (client, connection) =
            tokio_postgres::connect(&connection_string, tokio_postgres::NoTls)
                .await
                .context("Failed to connect to catalog database")?;

        tokio::task::Builder::new()
            .name("Catalog connection")
            .spawn(async move {
                if let Err(e) = connection.await {
                    tracing::error!("Connection error: {}", e);
                }
            })?;

        let pg_config = catalog_config.to_postgres_config();
        let executor = PostgresQueryExecutor::new(None, &pg_config).await?;
        let boxed_trait = Box::new(executor) as Box<dyn QueryExecutor>;

        Ok(Self {
            pg: Box::new(client),
            executor: Arc::new(boxed_trait),
        })
    }

    pub async fn run_migrations(&mut self) -> anyhow::Result<()> {
        run_migrations(&mut self.pg).await
    }

    pub fn get_executor(&self) -> Arc<Box<dyn QueryExecutor>> {
        self.executor.clone()
    }

    pub async fn create_peer(&self, peer: &Peer) -> anyhow::Result<i64> {
        let config_blob = {
            let config = peer.config.clone().context("invalid peer config")?;
            let mut buf = Vec::new();

            match config {
                Config::SnowflakeConfig(snowflake_config) => {
                    let config_len = snowflake_config.encoded_len();
                    buf.reserve(config_len);
                    snowflake_config.encode(&mut buf)?;
                }
                Config::BigqueryConfig(bigquery_config) => {
                    let config_len = bigquery_config.encoded_len();
                    buf.reserve(config_len);
                    bigquery_config.encode(&mut buf)?;
                }
                Config::MongoConfig(mongo_config) => {
                    let config_len = mongo_config.encoded_len();
                    buf.reserve(config_len);
                    mongo_config.encode(&mut buf)?;
                }
                Config::PostgresConfig(postgres_config) => {
                    let config_len = postgres_config.encoded_len();
                    buf.reserve(config_len);
                    postgres_config.encode(&mut buf)?;
                }
            };

            buf
        };

        let stmt = self
            .pg
            .prepare_typed(
                "INSERT INTO peers (name, type, options) VALUES ($1, $2, $3)",
                &[types::Type::TEXT, types::Type::INT4, types::Type::BYTEA],
            )
            .await?;

        self.pg
            .execute(&stmt, &[&peer.name, &peer.r#type, &config_blob])
            .await?;

        self.get_peer_id(&peer.name).await
    }

    async fn get_peer_id(&self, peer_name: &str) -> anyhow::Result<i64> {
        let id = self.get_peer_id_i32(peer_name).await?;
        Ok(id as i64)
    }

    // get peer id as i32
    pub async fn get_peer_id_i32(&self, peer_name: &str) -> anyhow::Result<i32> {
        let stmt = self
            .pg
            .prepare_typed("SELECT id FROM peers WHERE name = $1", &[types::Type::TEXT])
            .await?;

        self.pg
            .query_opt(&stmt, &[&peer_name])
            .await?
            .map(|row| row.get(0))
            .context("Failed to get peer id")
    }

    // get the database type for a given peer id
    pub async fn get_peer_type_for_id(&self, peer_id: i32) -> anyhow::Result<DbType> {
        let stmt = self
            .pg
            .prepare_typed("SELECT type FROM peers WHERE id = $1", &[types::Type::INT4])
            .await?;

        self.pg
            .query_opt(&stmt, &[&peer_id])
            .await?
            .map(|row| row.get(0))
            .map(|r#type| DbType::from_i32(r#type).unwrap()) // if row was inserted properly, this should never fail
            .context("Failed to get peer type")
    }

    pub async fn get_peers(&self) -> anyhow::Result<HashMap<String, Peer>> {
        let stmt = self
            .pg
            .prepare_typed("SELECT id, name, type, options FROM peers", &[])
            .await?;

        let rows = self.pg.query(&stmt, &[]).await?;

        let mut peers = HashMap::new();

        for row in rows {
            let name: String = row.get(1);
            let peer_type: i32 = row.get(2);
            let options: Vec<u8> = row.get(3);
            let db_type = DbType::from_i32(peer_type);

            let config = match db_type {
                Some(DbType::Snowflake) => {
                    let err = format!("unable to decode {} options for peer {}", "snowflake", name);
                    let snowflake_config =
                        pt::peers::SnowflakeConfig::decode(options.as_slice()).context(err)?;
                    Some(Config::SnowflakeConfig(snowflake_config))
                }
                Some(DbType::Bigquery) => {
                    let err = format!("unable to decode {} options for peer {}", "bigquery", name);
                    let bigquery_config =
                        pt::peers::BigqueryConfig::decode(options.as_slice()).context(err)?;
                    Some(Config::BigqueryConfig(bigquery_config))
                }
                Some(DbType::Mongo) => {
                    let err = format!("unable to decode {} options for peer {}", "mongo", name);
                    let mongo_config =
                        pt::peers::MongoConfig::decode(options.as_slice()).context(err)?;
                    Some(Config::MongoConfig(mongo_config))
                }
                Some(DbType::Postgres) => {
                    let err = format!("unable to decode {} options for peer {}", "postgres", name);
                    let postgres_config =
                        pt::peers::PostgresConfig::decode(options.as_slice()).context(err)?;
                    Some(Config::PostgresConfig(postgres_config))
                }
                None => None,
            };

            let peer = Peer {
                name: name.clone().to_lowercase(),
                r#type: peer_type,
                config,
            };

            peers.insert(name, peer);
        }

        Ok(peers)
    }

    pub async fn create_flow_job_entry(&self, job: &FlowJob) -> anyhow::Result<()> {
        let source_peer_id = self
            .get_peer_id_i32(&job.source_peer)
            .await
            .context("unable to get source peer id")?;
        let source_peer_dbtype = self
            .get_peer_type_for_id(source_peer_id)
            .await
            .context("unable to get source peer db type")?;
        let source_peer_schema_normalize = source_peer_dbtype != DbType::Bigquery;

        let destination_peer_id = self
            .get_peer_id_i32(&job.target_peer)
            .await
            .context("unable to get destination peer id")?;
        let destination_peer_dbtype = self
            .get_peer_type_for_id(destination_peer_id)
            .await
            .context("unable to get destination peer db type")?;
        let destination_peer_schema_normalize = destination_peer_dbtype != DbType::Bigquery;

        let stmt = self
            .pg
            .prepare_typed(
                "INSERT INTO flows (name, source_peer, destination_peer, description,
                     source_table_identifier, destination_table_identifier) VALUES ($1, $2, $3, $4, $5, $6)",
                &[types::Type::TEXT, types::Type::INT4, types::Type::INT4, types::Type::TEXT,
                 types::Type::TEXT, types::Type::TEXT],
            )
            .await?;

        for table_mapping in &job.table_mappings {
            let mut source_table_identifier_parts = table_mapping
                .source_table_identifier
                .split('.')
                .collect::<Vec<&str>>();
            if source_table_identifier_parts.len() == 1 && source_peer_schema_normalize {
                source_table_identifier_parts.insert(0, "public");
            }
            let mut destination_table_identifier_parts = table_mapping
                .target_table_identifier
                .split('.')
                .collect::<Vec<&str>>();
            if destination_table_identifier_parts.len() == 1 && destination_peer_schema_normalize {
                destination_table_identifier_parts.insert(0, "public");
            }

            let _rows = self
                .pg
                .execute(
                    &stmt,
                    &[
                        &job.name,
                        &source_peer_id,
                        &destination_peer_id,
                        &job.description,
                        &source_table_identifier_parts.join("."),
                        &destination_table_identifier_parts.join("."),
                    ],
                )
                .await?;
        }

        Ok(())
    }

    pub async fn update_workflow_id_for_flow_job(
        &self,
        flow_job_name: &str,
        workflow_id: &str,
    ) -> anyhow::Result<()> {
        let rows = self
            .pg
            .execute(
                "UPDATE FLOWS SET WORKFLOW_ID = $1 WHERE NAME = $2",
                &[&workflow_id, &flow_job_name],
            )
            .await?;
        if rows == 0 {
            return Err(anyhow!("unable to find metadata for flow"));
        }
        Ok(())
    }

    pub async fn get_workflow_id_for_flow_job(
        &self,
        flow_job_name: &str,
    ) -> anyhow::Result<Option<String>> {
        let rows = self
            .pg
            .query(
                "SELECT WORKFLOW_ID FROM FLOWS WHERE NAME = $1",
                &[&flow_job_name],
            )
            .await?;
        // currently multiple rows for a flow job exist in catalog, but all mapped to same workflow id
        // CHANGE LOGIC IF THIS ASSUMPTION CHANGES
        if rows.len() == 0 {
            tracing::info!("no workflow id found for flow job {}", flow_job_name);
            return Ok(None);
        }
        let first_row = rows.get(0).unwrap();
        Ok(Some(first_row.get(0)))
    }

    pub async fn delete_flow_job_entry(&self, flow_job_name: &str) -> anyhow::Result<()> {
        let rows = self
            .pg
            .execute("DELETE FROM FLOWS WHERE NAME = $1", &[&flow_job_name])
            .await?;
        if rows == 0 {
            return Err(anyhow!("unable to delete flow job metadata"));
        }
        Ok(())
    }
}
