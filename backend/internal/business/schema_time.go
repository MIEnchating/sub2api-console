package business

// Versioned RFC3339 numeric instant functions avoid the microsecond rounding of
// server timestamp types. The Gregorian day calculation supports year 0000,
// which Go accepts but PostgreSQL's timestamp input does not. Invalid dates
// return NULL, preserving the existing query behavior for legacy unknown dates.
const businessPostgresTimeSQL = `CREATE FUNCTION console_time_value_v1(time_text TEXT) RETURNS NUMERIC LANGUAGE plpgsql IMMUTABLE STRICT AS $$
DECLARE
 year_part BIGINT; month_part BIGINT; day_part BIGINT; hour_part BIGINT; minute_part BIGINT; second_part BIGINT;
 offset_seconds BIGINT:=0; fraction_part NUMERIC:=0; era_part BIGINT; year_in_era BIGINT; day_in_year BIGINT; day_in_era BIGINT; days_part BIGINT; max_day BIGINT;
BEGIN
 IF time_text !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.,][0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' THEN RETURN NULL; END IF;
 IF right(time_text,1)<>'Z' AND right(time_text,1) !~ '^[0-9]$' THEN RETURN NULL; END IF;
 year_part:=substring(time_text,1,4)::BIGINT; month_part:=substring(time_text,6,2)::BIGINT; day_part:=substring(time_text,9,2)::BIGINT;
 hour_part:=substring(time_text,12,2)::BIGINT; minute_part:=substring(time_text,15,2)::BIGINT; second_part:=substring(time_text,18,2)::BIGINT;
 IF month_part<1 OR month_part>12 OR hour_part>23 OR minute_part>59 OR second_part>59 THEN RETURN NULL; END IF;
 max_day:=CASE WHEN month_part=2 THEN CASE WHEN mod(year_part,400)=0 OR (mod(year_part,4)=0 AND mod(year_part,100)<>0) THEN 29 ELSE 28 END WHEN month_part IN (4,6,9,11) THEN 30 ELSE 31 END;
 IF day_part<1 OR day_part>max_day THEN RETURN NULL; END IF;
 IF right(time_text,1)<>'Z' THEN
  IF substring(right(time_text,6),2,2)::BIGINT>24 OR right(time_text,2)::BIGINT>60 THEN RETURN NULL; END IF;
  offset_seconds:=(substring(right(time_text,6),2,2)::BIGINT*3600+right(time_text,2)::BIGINT*60)*CASE WHEN left(right(time_text,6),1)='-' THEN -1 ELSE 1 END;
 END IF;
 fraction_part:=COALESCE(rpad(substring(substring(time_text FROM '[.,][0-9]+'),2,9),9,'0')::NUMERIC,0)/1000000000;
 year_part:=year_part-CASE WHEN month_part<=2 THEN 1 ELSE 0 END;
 era_part:=floor(year_part::NUMERIC/400); year_in_era:=year_part-era_part*400;
 day_in_year:=floor((153*(month_part+CASE WHEN month_part>2 THEN -3 ELSE 9 END)+2)::NUMERIC/5)+day_part-1;
 day_in_era:=year_in_era*365+floor(year_in_era::NUMERIC/4)-floor(year_in_era::NUMERIC/100)+day_in_year;
 days_part:=era_part*146097+day_in_era-719468;
 RETURN days_part*86400+hour_part*3600+minute_part*60+second_part-offset_seconds+fraction_part;
END $$`

const businessMySQLTimeSQL = `CREATE FUNCTION console_time_value_v1(time_text LONGTEXT) RETURNS DECIMAL(30,9) DETERMINISTIC NO SQL
BEGIN
 DECLARE year_part,month_part,day_part,hour_part,minute_part,second_part,offset_seconds,era_part,year_in_era,day_in_year,day_in_era,days_part,max_day BIGINT DEFAULT 0;
 DECLARE fraction_part DECIMAL(10,9) DEFAULT 0;
 IF time_text IS NULL OR NOT REGEXP_LIKE(time_text,'^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.,][0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$','c') THEN RETURN NULL; END IF;
 IF RIGHT(time_text,1)<>'Z' AND NOT REGEXP_LIKE(RIGHT(time_text,1),'^[0-9]$','c') THEN RETURN NULL; END IF;
 SET year_part=CAST(SUBSTRING(time_text,1,4) AS SIGNED),month_part=CAST(SUBSTRING(time_text,6,2) AS SIGNED),day_part=CAST(SUBSTRING(time_text,9,2) AS SIGNED);
 SET hour_part=CAST(SUBSTRING(time_text,12,2) AS SIGNED),minute_part=CAST(SUBSTRING(time_text,15,2) AS SIGNED),second_part=CAST(SUBSTRING(time_text,18,2) AS SIGNED);
 IF month_part<1 OR month_part>12 OR hour_part>23 OR minute_part>59 OR second_part>59 THEN RETURN NULL; END IF;
 SET max_day=CASE WHEN month_part=2 THEN CASE WHEN MOD(year_part,400)=0 OR (MOD(year_part,4)=0 AND MOD(year_part,100)<>0) THEN 29 ELSE 28 END WHEN month_part IN (4,6,9,11) THEN 30 ELSE 31 END;
 IF day_part<1 OR day_part>max_day THEN RETURN NULL; END IF;
 IF RIGHT(time_text,1)<>'Z' THEN
  IF CAST(SUBSTRING(RIGHT(time_text,6),2,2) AS SIGNED)>24 OR CAST(RIGHT(time_text,2) AS SIGNED)>60 THEN RETURN NULL; END IF;
  SET offset_seconds=(CAST(SUBSTRING(RIGHT(time_text,6),2,2) AS SIGNED)*3600+CAST(RIGHT(time_text,2) AS SIGNED)*60)*CASE WHEN LEFT(RIGHT(time_text,6),1)='-' THEN -1 ELSE 1 END;
 END IF;
 SET fraction_part=CAST(COALESCE(RPAD(SUBSTRING(REGEXP_SUBSTR(time_text,'[.,][0-9]+'),2,9),9,'0'),'0') AS DECIMAL(20,9))/1000000000;
 SET year_part=year_part-CASE WHEN month_part<=2 THEN 1 ELSE 0 END;
 SET era_part=FLOOR(year_part/400),year_in_era=year_part-era_part*400;
 SET day_in_year=FLOOR((153*(month_part+CASE WHEN month_part>2 THEN -3 ELSE 9 END)+2)/5)+day_part-1;
 SET day_in_era=year_in_era*365+FLOOR(year_in_era/4)-FLOOR(year_in_era/100)+day_in_year;
 SET days_part=era_part*146097+day_in_era-719468;
 RETURN days_part*86400+hour_part*3600+minute_part*60+second_part-offset_seconds+fraction_part;
END`
