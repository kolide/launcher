select checksum, COUNT(*) from local_entry GROUP BY checksum;


select distinct le.inode, le.filename, le.modified, le.size, le.checksum,
       CASE WHEN ce.doc_type = 1 THEN 'non_google'
            WHEN ce.doc_type = 6 THEN 'google_doc'
            WHEN ce.doc_type = 4 THEN 'google_sheet'
			WHEN ce.doc_type = 2 THEN 'google_slide'
            ELSE NULL END AS doc_type
 from local_entry le, cloud_entry ce using (checksum);
